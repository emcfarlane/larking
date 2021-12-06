package larking

import (
	"context"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/emcfarlane/larking/starlarkblob"
	"github.com/emcfarlane/larking/starlarkdocstore"
	"github.com/emcfarlane/larking/starlarkerrors"
	"github.com/emcfarlane/larking/starlarknethttp"
	"github.com/emcfarlane/larking/starlarkpubsub"
	"github.com/emcfarlane/larking/starlarkruntimevar"
	"github.com/emcfarlane/larking/starlarksql"
	"github.com/emcfarlane/starlarkassert"
	"github.com/emcfarlane/starlarkproto"
	starlarkjson "go.starlark.net/lib/json"
	starlarkmath "go.starlark.net/lib/math"
	starlarktime "go.starlark.net/lib/time"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// Bootstrap...
// TODO: xDS

func NewGlobals() starlark.StringDict {
	return starlark.StringDict{
		"struct": starlark.NewBuiltin("struct", starlarkstruct.Make),
	}
}

type Loader struct {
	mu    sync.RWMutex
	store map[string]starlark.StringDict
}

// NewLoader creates a starlark loader with modules that support loading themselves as
// a starlarkstruct.Module:
//     load("module.star", "module")
func NewLoader() (*Loader, error) {
	thread := new(starlark.Thread)
	assert, err := starlarkassert.LoadAssertModule(thread)
	if err != nil {
		return nil, err
	}
	// SetReporter to default to threads default.

	store := map[string]starlark.StringDict{
		"assert.star": assert,
	}

	// create mux
	mux, err := NewMux()
	if err != nil {
		return nil, err
	}

	modules := []*starlarkstruct.Module{
		starlarkblob.NewModule(),
		starlarkdocstore.NewModule(),
		starlarkerrors.NewModule(),
		starlarknethttp.NewModule(),
		starlarkpubsub.NewModule(),
		starlarkruntimevar.NewModule(),
		starlarksql.NewModule(),
		starlarkproto.NewModule(protoregistry.GlobalFiles),

		// TODO: move to thread variables?
		NewModule(mux),

		// starlark native modules
		starlarkjson.Module,
		starlarkmath.Module,
		starlarktime.Module,
	}
	for _, module := range modules {
		dict := make(starlark.StringDict, len(module.Members)+1)
		for key, val := range module.Members {
			dict[key] = val
		}
		dict[module.Name] = module
		store[module.Name+".star"] = dict
	}

	return &Loader{
		store: store,
	}, nil
}

func (l *Loader) Load(thread *starlark.Thread, module string) (starlark.StringDict, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if e, ok := l.store[module]; ok {
		return e, nil
	}
	// TODO: an error of sorts.
	return nil, os.ErrNotExist
}

func NewModule(mux *Mux) *starlarkstruct.Module {
	s := NewStarlark(mux)
	return &starlarkstruct.Module{
		Name: "grpc",
		Members: starlark.StringDict{
			"dial":    starlark.NewBuiltin("grpc.dial", s.Dial),
			"service": starlark.NewBuiltin("grpc.service", s.Service),
		},
	}
}

type Starlark struct {
	mux *Mux
}

func NewStarlark(mux *Mux) *Starlark {
	return &Starlark{mux: mux}
}

// Dial accepts an address and optional
func (s *Starlark) Dial(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var target string
	if err := starlark.UnpackPositionalArgs("grpc.dial", args, kwargs, 1, &target); err != nil {
		return nil, err
	}

	ctx, ok := thread.Local("context").(context.Context)
	if !ok {
		ctx = context.Background()
	}

	// TODO: handle security, opts.
	dialCtx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()
	cc, err := grpc.DialContext(dialCtx, target, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	if err := s.mux.RegisterConn(ctx, cc); err != nil {
		return nil, err
	}

	// TODO?
	return starlark.None, nil
}

// Service returns a starlark Value with callable attributes to all the methods
// of the service.
func (s *Starlark) Service(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackPositionalArgs("grpc.service", args, kwargs, 1, &name); err != nil {
		return nil, err
	}

	pfx := "/" + name
	if state := s.mux.loadState(); state != nil {
		for method := range state.handlers {
			if strings.HasPrefix(method, pfx) {
				return &StarlarkService{
					mux:  s.mux,
					name: name,
				}, nil
			}

		}
	}
	return nil, status.Errorf(codes.NotFound, "unknown service: %s", name)
}

type StarlarkService struct {
	mux  *Mux
	name string
}

func (s *StarlarkService) String() string        { return s.name }
func (s *StarlarkService) Type() string          { return "grpc.service" }
func (s *StarlarkService) Freeze()               {} // immutable
func (s *StarlarkService) Truth() starlark.Bool  { return starlark.True }
func (s *StarlarkService) Hash() (uint32, error) { return 0, nil }

// HasAttrs with each one being callable.
func (s *StarlarkService) Attr(name string) (starlark.Value, error) {
	m := "/" + s.name + "/" + name
	hd, err := s.mux.loadState().pickMethodHandler(m)
	if err != nil {
		return nil, err
	}
	return &StarlarkMethod{
		mux: s.mux,
		hd:  hd,
	}, nil
}
func (s *StarlarkService) AttrNames() []string {
	var attrs []string

	pfx := "/" + s.name + "/"
	for method := range s.mux.loadState().handlers {
		if strings.HasPrefix(method, pfx) {
			attrs = append(attrs, strings.TrimPrefix(method, pfx))
		}
	}

	return attrs
}

type streamStar struct {
	ctx        context.Context
	method     string
	sentHeader bool
	header     metadata.MD
	trailer    metadata.MD

	starArgs   starlark.Tuple
	starKwargs []starlark.Tuple

	args    []proto.Message
	replies []proto.Message
}

func (s *streamStar) SetHeader(md metadata.MD) error {
	if !s.sentHeader {
		s.header = metadata.Join(s.header, md)
	}
	return nil

}
func (s *streamStar) SendHeader(md metadata.MD) error {
	if s.sentHeader {
		return nil // already sent?
	}
	// TODO: write header?
	s.sentHeader = true
	return nil
}

func (s *streamStar) SetTrailer(md metadata.MD) {
	s.sentHeader = true
	s.trailer = metadata.Join(s.trailer, md)
}

func (s *streamStar) Context() context.Context {
	ctx := newIncomingContext(s.ctx, nil) // TODO: remove me?
	sts := &serverTransportStream{s, s.method}
	return grpc.NewContextWithServerTransportStream(ctx, sts)
}

func (s *streamStar) SendMsg(m interface{}) error {
	reply := m.(proto.Message)
	if len(s.replies) > 0 {
		// TODO: streaming.
		return io.EOF
	}
	s.replies = append(s.replies, reply)
	return nil
}

func (s *streamStar) RecvMsg(m interface{}) error {
	args := m.(proto.Message)
	msg := args.ProtoReflect()

	if len(s.args) > 0 {
		// TODO: streaming.
		return io.EOF
	}

	// Capture starlark arguments.
	_, err := starlarkproto.NewMessage(msg, s.starArgs, s.starKwargs)
	if err != nil {
		return err
	}
	s.args = append(s.args, args)
	return nil
}

type StarlarkMethod struct {
	mux *Mux
	hd  *handler
	// Callable
}

func (s *StarlarkMethod) String() string        { return s.hd.method }
func (s *StarlarkMethod) Type() string          { return "grpc.method" }
func (s *StarlarkMethod) Freeze()               {} // immutable
func (s *StarlarkMethod) Truth() starlark.Bool  { return starlark.True }
func (s *StarlarkMethod) Hash() (uint32, error) { return 0, nil }
func (s *StarlarkMethod) Name() string          { return "" }
func (s *StarlarkMethod) CallInternal(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx, ok := thread.Local("context").(context.Context)
	if !ok {
		ctx = context.Background()
	}

	opts := &s.mux.opts
	stream := &streamStar{
		ctx:        ctx,
		method:     s.hd.method,
		starArgs:   args,
		starKwargs: kwargs,
	}

	if err := s.hd.handler(opts, stream); err != nil {
		return nil, err
	}

	if len(stream.replies) != 1 {
		//
		return nil, io.EOF
	}

	return starlarkproto.NewMessage(stream.replies[0].ProtoReflect(), nil, nil)
}

// finxPrefix assumes sorted arrays of keys
func findPrefix(line string, depth int, pfx string, keyss ...[]string) (c []string) {
	pfx = strings.TrimSpace(pfx) // ignore any whitespacing
	for _, keys := range keyss {
		i := sort.SearchStrings(keys, pfx)
		j := i
		for ; j < len(keys); j++ {
			if !strings.HasPrefix(keys[j], pfx) {
				break
			}
		}
		c = append(c, keys[i:j]...)
	}
	if len(keyss) > 1 {
		sort.Strings(c)
	}

	// Add line start
	for i := range c {
		c[i] = line[:depth] + c[i]
	}
	return c
}

// An Args is a starlark Callable with arguments.
type Args interface {
	starlark.Callable
	ArgNames() []string
}

// Completer is an experimental autocompletion for starlark lines.
// TODO: drop and switch to a proper language server.
type Completer struct {
	starlark.StringDict
}

type typ int

const (
	unknown typ = iota - 1
	root        //
	dot         // .
	brack       // []
	paren       // ()
	brace       // {}
)

func (t typ) String() string {
	switch t {
	case dot:
		return "."
	case brack:
		return "["
	case paren:
		return "("
	case brace:
		return "{"
	default:
		return "?"
	}
}

func enclosed(line string) (typ, int) {
	k := len(line)
	var parens, bracks, braces int
	for size := 0; k > 0; k -= size {
		var r rune
		r, size = utf8.DecodeLastRuneInString(line[:k])
		switch r {
		case '(':
			parens += 1
		case ')':
			parens -= 1
		case '[':
			bracks += 1
		case ']':
			bracks -= 1
		case '{':
			braces += 1
		case '}':
			braces -= 1
		}
		if parens > 0 {
			return paren, k - size
		}
		if bracks > 0 {
			return brack, k - size
		}
		if braces > 0 {
			return brace, k - size
		}
	}
	return unknown, -1
}

// Complete tries to resolve a starlark line variable to global named values.
// TODO: use a proper parser to resolve values.
func (c Completer) Complete(line string) (values []string) {
	if strings.Count(line, " ") == len(line) {
		// tab complete indent
		return []string{strings.Repeat(" ", (len(line)/4)*4+4)}
	}

	type x struct {
		typ   typ
		value string
		depth int
	}

	var xs []x

	i := len(line)
	j := i

Loop:
	for size := 0; i > 0; i -= size {
		var r rune
		switch r, size = utf8.DecodeLastRuneInString(line[:i]); r {
		case '.': // attr
			xs = append(xs, x{dot, line[i:j], i})
		case '[': // index
			xs = append(xs, x{brack, line[i:j], i})
		case '(': // functions
			xs = append(xs, x{paren, line[i:j], i})
		case ' ', ',':
			typ, k := enclosed(line[:i-size])

			// Use ArgNames as possible completion
			if typ == paren {
				xs = append(xs, x{typ, line[i:j], i})
				i, j = k, k
				continue // loop
			}

			break Loop
		case ';', '=', '{', '}':
			break Loop // EOF
		default:
			continue // capture
		}
		j = i - size
	}
	xs = append(xs, x{root, line[i:j], i})

	var cursor starlark.Value
	for i := len(xs) - 1; i >= 0; i-- {
		x := xs[i]

		switch x.typ {
		case root:
			if i == 0 {
				keys := [][]string{c.Keys(), starlark.Universe.Keys()}
				return findPrefix(line, x.depth, x.value, keys...)
			}

			if g := c.StringDict[x.value]; g != nil {
				cursor = g
			} else if u := starlark.Universe[x.value]; u != nil {
				cursor = u
			}
		case dot:
			v, ok := cursor.(starlark.HasAttrs)
			if !ok {
				return
			}

			if i == 0 {
				return findPrefix(line, x.depth, x.value, v.AttrNames())
			}

			p, err := v.Attr(x.value)
			if p == nil || err != nil {
				return
			}
			cursor = p
		case brack:
			if i != 0 {
				// TODO: resolve arg? fmt.Printf("TODO: resolve arg %s\n", x.value)
				return
			}

			if strings.HasPrefix(x.value, "\"") {
				v, ok := cursor.(starlark.IterableMapping)
				if !ok {
					return
				}

				iter := v.Iterate()
				var keys []string
				var p starlark.Value
				for iter.Next(&p) {
					s, ok := starlark.AsString(p)
					if !ok {
						continue // skip
					}
					keys = append(keys, strconv.Quote(s)+"]")
				}
				return findPrefix(line, x.depth, x.value, keys)
			}
			keys := [][]string{c.Keys(), starlark.Universe.Keys()}
			return findPrefix(line, x.depth, x.value, keys...)

		case paren:
			if i != 0 {
				return // Functions aren't evalutated
			}

			keys := [][]string{c.Keys(), starlark.Universe.Keys()}
			v, ok := cursor.(Args)
			if ok {
				args := v.ArgNames()
				for i := range args {
					args[i] = args[i] + " = "
				}
				keys = append(keys, args)
			}

			return findPrefix(line, x.depth, x.value, keys...)
		default:
			return
		}
	}
	return
}
