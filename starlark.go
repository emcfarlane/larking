package larking

import (
	"context"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/emcfarlane/larking/starlarkproto"
	"github.com/emcfarlane/larking/starlarkthread"
	"go.starlark.net/starlark"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func (m *Mux) String() string        { return "mux" }
func (m *Mux) Type() string          { return "mux" }
func (m *Mux) Freeze()               {} // immutable
func (m *Mux) Truth() starlark.Bool  { return starlark.True }
func (m *Mux) Hash() (uint32, error) { return 0, nil }

var starlarkMuxMethods = map[string]*starlark.Builtin{
	"service": starlark.NewBuiltin("mux.service", OpenStarlarkService),
}

func (m *Mux) Attr(name string) (starlark.Value, error) {
	b := starlarkMuxMethods[name]
	if b == nil {
		return nil, nil
	}
	return b.BindReceiver(m), nil
}
func (v *Mux) AttrNames() []string {
	names := make([]string, 0, len(starlarkMuxMethods))
	for name := range starlarkMuxMethods {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

type StarlarkService struct {
	mux  *Mux
	name string
}

func OpenStarlarkService(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	mux := b.Receiver().(*Mux)

	var name string
	if err := starlark.UnpackPositionalArgs("mux.service", args, kwargs, 1, &name); err != nil {
		return nil, err
	}
	return mux.OpenStarlarkService(name)
}

func (m *Mux) OpenStarlarkService(name string) (*StarlarkService, error) {
	pfx := "/" + name
	if state := m.loadState(); state != nil {
		for method := range state.handlers {
			if strings.HasPrefix(method, pfx) {
				return &StarlarkService{
					mux:  m,
					name: name,
				}, nil
			}

		}
	}
	return nil, status.Errorf(codes.NotFound, "unknown service: %s", name)
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

	if hd.descriptor.IsStreamingClient() || hd.descriptor.IsStreamingServer() {
		ss := &StarlarkStream{
			mux: s.mux,
			hd:  hd,
		}

		return ss, nil
	}

	return &StarlarkUnary{
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
	sort.Strings(attrs)
	return attrs
}

type starlarkStream struct {
	ctx        context.Context
	method     string
	sentHeader bool
	header     metadata.MD
	trailer    metadata.MD
	ins        chan func(proto.Message) error
	outs       chan func(proto.Message) error
}

func (s *starlarkStream) SetHeader(md metadata.MD) error {
	if !s.sentHeader {
		s.header = metadata.Join(s.header, md)
	}
	return nil

}
func (s *starlarkStream) SendHeader(md metadata.MD) error {
	if s.sentHeader {
		return nil // already sent?
	}
	// TODO: write header?
	s.sentHeader = true
	return nil
}

func (s *starlarkStream) SetTrailer(md metadata.MD) {
	s.sentHeader = true
	s.trailer = metadata.Join(s.trailer, md)
}

func (s *starlarkStream) Context() context.Context {
	ctx := newIncomingContext(s.ctx, nil) // TODO: remove me?
	sts := &serverTransportStream{s, s.method}
	return grpc.NewContextWithServerTransportStream(ctx, sts)
}

func (s *starlarkStream) SendMsg(m interface{}) error {
	reply := m.(proto.Message)
	select {
	case fn := <-s.outs:
		return fn(reply)
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

func (s *starlarkStream) RecvMsg(m interface{}) error {
	args := m.(proto.Message)
	//msg := args.ProtoReflect()

	select {
	case fn := <-s.ins:
		return fn(args)
	case <-s.ctx.Done():
		return s.ctx.Err()
	}

}

type StarlarkUnary struct {
	mux *Mux
	hd  *handler
}

func (s *StarlarkUnary) String() string        { return s.hd.method }
func (s *StarlarkUnary) Type() string          { return "grpc.unary_method" }
func (s *StarlarkUnary) Freeze()               {} // immutable
func (s *StarlarkUnary) Truth() starlark.Bool  { return starlark.True }
func (s *StarlarkUnary) Hash() (uint32, error) { return 0, nil }
func (s *StarlarkUnary) Name() string          { return "" }
func (s *StarlarkUnary) CallInternal(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := starlarkthread.Context(thread)
	opts := &s.mux.opts

	// Buffer channels one message for unary.
	stream := &starlarkStream{
		ctx:    ctx,
		method: s.hd.method,
		ins:    make(chan func(proto.Message) error, 1),
		outs:   make(chan func(proto.Message) error, 1),
	}

	stream.ins <- func(msg proto.Message) error {
		arg := msg.ProtoReflect()

		// Capture starlark arguments.
		_, err := starlarkproto.NewMessage(arg, args, kwargs)
		return err
	}

	var rsp *starlarkproto.Message
	stream.outs <- func(msg proto.Message) error {
		arg := msg.ProtoReflect()

		val, err := starlarkproto.NewMessage(arg, nil, nil)
		rsp = val
		return err
	}

	if err := s.hd.handler(opts, stream); err != nil {
		return nil, err
	}
	return rsp, nil
}

type StarlarkStream struct {
	mux *Mux
	hd  *handler

	once   sync.Once
	cancel func()
	stream *starlarkStream

	onceErr sync.Once
	err     error
}

func (s *StarlarkStream) setErr(err error) {
	s.onceErr.Do(func() { s.err = err })
}
func (s *StarlarkStream) getErr() error {
	s.setErr(nil) // blow away onceErr
	return s.err
}

// init lazy initializes the streaming handler.
func (s *StarlarkStream) init(thread *starlark.Thread) error {
	ctx := starlarkthread.Context(thread)
	opts := &s.mux.opts

	s.once.Do(func() {
		if err := starlarkthread.AddResource(thread, s); err != nil {
			s.setErr(err)
			return
		}

		ctx, cancel := context.WithCancel(ctx)
		s.cancel = cancel
		s.stream = &starlarkStream{
			ctx:    ctx,
			method: s.hd.method,
			ins:    make(chan func(proto.Message) error),
			outs:   make(chan func(proto.Message) error),
		}

		// Start the handler
		go func() {
			s.onceErr.Do(func() {
				s.err = s.hd.handler(opts, s.stream)
			})
			cancel()
		}()
	})
	if s.stream == nil || s.stream.ctx.Err() != nil {
		return io.EOF // cancelled before starting or cancelled
	}
	return nil
}

func (s *StarlarkStream) String() string        { return s.hd.method }
func (s *StarlarkStream) Type() string          { return "grpc.stream_method" }
func (s *StarlarkStream) Freeze()               {} // immutable???
func (s *StarlarkStream) Truth() starlark.Bool  { return starlark.True }
func (s *StarlarkStream) Hash() (uint32, error) { return 0, nil }
func (s *StarlarkStream) Name() string          { return "" }

func (s *StarlarkStream) Close() error {
	s.once.Do(func() {}) // blow the once away
	if s.cancel == nil {
		return nil // never started
	}
	s.cancel()
	return s.getErr()
}

func (s *StarlarkStream) Attr(name string) (starlark.Value, error) {
	b := starlarkStreamMethods[name]
	if b == nil {
		return nil, nil
	}
	return b.BindReceiver(s), nil
}
func (v *StarlarkStream) AttrNames() []string {
	names := make([]string, 0, len(starlarkStreamMethods))
	for name := range starlarkStreamMethods {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

var starlarkStreamMethods = map[string]*starlark.Builtin{
	"recv": starlark.NewBuiltin("grpc.method.recv", starlarkStreamRecv),
	"send": starlark.NewBuiltin("grpc.method.send", starlarkStreamSend),
}

type starlarkResponse struct {
	val starlark.Value
	err error
}

func promiseResponse(
	ctx context.Context, args starlark.Tuple, kwargs []starlark.Tuple,
) (func(proto.Message) error, <-chan starlarkResponse) {
	ch := make(chan starlarkResponse)

	return func(msg proto.Message) error {
		arg := msg.ProtoReflect()

		val, err := starlarkproto.NewMessage(arg, args, kwargs)
		select {
		case ch <- starlarkResponse{val: val, err: err}:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}, ch
}

func starlarkStreamRecv(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := starlarkthread.Context(thread)
	s := b.Receiver().(*StarlarkStream)
	if err := s.init(thread); err != nil {
		return nil, err
	}

	if err := starlark.UnpackPositionalArgs("grpc.method.stream", args, kwargs, 0); err != nil {
		return nil, err
	}

	fn, ch := promiseResponse(ctx, nil, nil)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.stream.ctx.Done():
		return nil, s.getErr()
	case s.stream.outs <- fn:
		rsp := <-ch
		return rsp.val, rsp.err
	}
}
func starlarkStreamSend(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := starlarkthread.Context(thread)
	s := b.Receiver().(*StarlarkStream)
	if err := s.init(thread); err != nil {
		return nil, err
	}

	fn, ch := promiseResponse(ctx, args, kwargs)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.stream.ctx.Done():
		return nil, s.getErr()
	case s.stream.ins <- fn:
		rsp := <-ch
		return starlark.None, rsp.err
	}
}
