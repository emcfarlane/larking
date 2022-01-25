package larking

import (
	"context"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/emcfarlane/larking/starlarkblob"
	"github.com/emcfarlane/larking/starlarkdocstore"
	"github.com/emcfarlane/larking/starlarkerrors"
	"github.com/emcfarlane/larking/starlarknethttp"
	"github.com/emcfarlane/larking/starlarkpubsub"
	"github.com/emcfarlane/larking/starlarkruntimevar"
	"github.com/emcfarlane/larking/starlarksql"
	"github.com/emcfarlane/larking/starlarkstruct"
	"github.com/emcfarlane/larking/starlarkthread"
	"github.com/emcfarlane/starlarkassert"
	"github.com/emcfarlane/starlarkproto"
	starlarkjson "go.starlark.net/lib/json"
	starlarkmath "go.starlark.net/lib/math"
	starlarktime "go.starlark.net/lib/time"
	"go.starlark.net/resolve"
	"go.starlark.net/starlark"
	"gocloud.dev/blob"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// Bootstrap...
// TODO: xDS

func init() {
	resolve.AllowSet = true
	resolve.AllowRecursion = true
}

func NewGlobals() starlark.StringDict {
	return starlark.StringDict{
		"struct": starlark.NewBuiltin("struct", starlarkstruct.Make),
		"module": starlark.NewBuiltin("module", starlarkstruct.MakeModule),
	}
}

type Loader struct {
	mu    sync.Mutex
	store map[string]starlark.StringDict
	bkt   *blob.Bucket
}

// NewLoader creates a starlark loader with modules that support loading themselves as
// a starlarkstruct.Module:
//     load("module.star", "module")
func NewLoader(ctx context.Context, urlstr string) (*Loader, error) {
	bkt, err := blob.OpenBucket(ctx, urlstr)
	if err != nil {
		return nil, err
	}

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
		bkt:   bkt,
	}, nil
}

func (l *Loader) Load(thread *starlark.Thread, filename string) (starlark.StringDict, error) {
	l.mu.Lock()
	if e, ok := l.store[filename]; ok {
		l.mu.Unlock()
		return e, nil
	}
	l.mu.Unlock()

	// TODO: singleflight.
	// TODO: third-party module loading.
	ctx := starlarkthread.Context(thread)

	src, err := l.bkt.ReadAll(ctx, filename)
	if err != nil {
		return nil, err
	}

	module, err := starlark.ExecFile(thread, filename, src, nil)
	if err != nil {
		return nil, err
	}
	l.mu.Lock()
	l.store[filename] = module
	l.mu.Unlock()
	return module, nil
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

	ctx := starlarkthread.Context(thread)

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
	ctx := starlarkthread.Context(thread)

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
