package larking

import (
	"context"
	"fmt"
	"strings"

	"github.com/emcfarlane/starlarkproto"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/dynamicpb"
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

	// TODO: opts?
	cc, err := grpc.DialContext(ctx, target)
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

	fmt.Println("grpc.service", name)

	pfx := "/" + name
	for method := range s.mux.loadState().methods {
		if strings.HasPrefix(method, pfx) {
			return &StarlarkService{
				mux:  s.mux,
				name: name,
			}, nil
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
	mc, err := s.mux.loadState().pickMethodConn(m)
	if err != nil {
		return nil, err
	}
	return &StarlarkMethod{
		mc: mc,
	}, nil
}
func (s *StarlarkService) AttrNames() []string {
	var attrs []string

	pfx := "/" + s.name + "/"
	for method := range s.mux.loadState().methods {
		if strings.HasPrefix(method, pfx) {
			attrs = append(attrs, strings.TrimPrefix(method, pfx))
		}
	}

	return attrs
}

type StarlarkMethod struct {
	mc methodConn
	//method string
	// Callable
}

func (s *StarlarkMethod) String() string        { return s.mc.name }
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

	// Check arg is a proto.Message of the form required.
	argsDesc := s.mc.desc.Input()
	replyDesc := s.mc.desc.Output()

	argsPb := dynamicpb.NewMessage(argsDesc)
	replyPb := dynamicpb.NewMessage(replyDesc)

	// Capture starlark arguments.
	_, err := starlarkproto.NewMessage(argsPb, args, kwargs)
	if err != nil {
		return nil, err
	}

	var header, trailer metadata.MD
	if err := s.mc.cc.Invoke(
		ctx,
		s.mc.name,
		argsPb, replyPb,
		grpc.Header(&header),
		grpc.Trailer(&trailer),
	); err != nil {
		//setOutgoingHeader(w.Header(), header, trailer)
		return nil, err
	}
	// TODO: how to provide header details?

	return starlarkproto.NewMessage(replyPb, nil, nil)
}
