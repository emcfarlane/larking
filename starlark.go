package larking

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/emcfarlane/starlarkproto"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

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
