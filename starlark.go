package larking

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/emcfarlane/larking/starext"
	"github.com/emcfarlane/larking/starlarkproto"
	"github.com/emcfarlane/larking/starlarkthread"
	"github.com/go-logr/logr"
	"go.starlark.net/starlark"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

func (m *Mux) String() string        { return "mux" }
func (m *Mux) Type() string          { return "mux" }
func (m *Mux) Freeze()               {} // immutable
func (m *Mux) Truth() starlark.Bool  { return starlark.True }
func (m *Mux) Hash() (uint32, error) { return 0, nil }

type muxAttr func(m *Mux) starlark.Value

var muxMethods = map[string]muxAttr{
	"service": func(m *Mux) starlark.Value {
		return starext.MakeMethod(m, "service", m.openStarlarkService)
	},
	"register_service": func(m *Mux) starlark.Value {
		return starext.MakeMethod(m, "register", m.registerStarlarkService)
	},
}

func (m *Mux) Attr(name string) (starlark.Value, error) {
	if a := muxMethods[name]; a != nil {
		return a(m), nil
	}
	return nil, nil
}
func (v *Mux) AttrNames() []string {
	names := make([]string, 0, len(muxMethods))
	for name := range muxMethods {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

type StarlarkService struct {
	mux  *Mux
	name string
}

func (m *Mux) openStarlarkService(_ *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackPositionalArgs(fnname, args, nil, 1, &name); err != nil {
		return nil, err
	}

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

func starlarkUnimplemented(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return nil, status.Errorf(codes.Unimplemented, "method %s not implemented", fnname)
}

func createStarlarkHandler(
	parent *starlark.Thread,
	fn starlark.Callable,
	sd protoreflect.ServiceDescriptor,
	md protoreflect.MethodDescriptor,
) *handler {

	argsDesc := md.Input()
	replyDesc := md.Output()

	method := fmt.Sprintf("/%s/%s", sd.FullName(), md.Name())

	isClientStream := md.IsStreamingClient()
	isServerStream := md.IsStreamingServer()
	if isClientStream || isServerStream {
		//sd := &grpc.StreamDesc{
		//	ServerStreams: md.IsStreamingServer(),
		//	ClientStreams: md.IsStreamingClient(),
		//}
		info := &grpc.StreamServerInfo{
			FullMethod:     method,
			IsClientStream: isClientStream,
			IsServerStream: isServerStream,
		}

		// TODO: check not mutated.
		//globals := starlib.NewGlobals()

		fn := func(_ interface{}, stream grpc.ServerStream) (err error) {
			ctx := stream.Context()

			args := dynamicpb.NewMessage(argsDesc)
			//reply := dynamicpb.NewMessage(replyDesc)

			if err := stream.RecvMsg(args); err != nil {
				return err
			}

			if md, ok := metadata.FromIncomingContext(ctx); ok {
				ctx = metadata.NewOutgoingContext(ctx, md)
			}

			// build thread
			l := logr.FromContextOrDiscard(ctx)
			thread := &starlark.Thread{
				Name: parent.Name,
				Print: func(_ *starlark.Thread, msg string) {
					l.Info(msg, "thread", parent.Name)
				},
				Load: parent.Load,
			}
			starlarkthread.SetContext(thread, ctx)
			close := starlarkthread.WithResourceStore(thread)
			defer func() {
				if cerr := close(); err == nil {
					err = cerr
				}
			}()

			// TODO: streams.
			return fmt.Errorf("unimplemented")
		}

		h := func(opts *muxOptions, stream grpc.ServerStream) error {
			return opts.stream(nil, stream, info, fn)
		}

		return &handler{
			method:     method,
			descriptor: md,
			handler:    h,
		}
	} else {
		info := &grpc.UnaryServerInfo{
			Server:     nil,
			FullMethod: method,
		}
		fn := func(ctx context.Context, args interface{}) (reply interface{}, err error) {

			if md, ok := metadata.FromIncomingContext(ctx); ok {
				ctx = metadata.NewOutgoingContext(ctx, md)
			}

			l := logr.FromContextOrDiscard(ctx)
			thread := &starlark.Thread{
				Name: parent.Name,
				Print: func(_ *starlark.Thread, msg string) {
					l.Info(msg, "thread", parent.Name)
				},
				Load: parent.Load,
			}
			starlarkthread.SetContext(thread, ctx)
			close := starlarkthread.WithResourceStore(thread)
			defer func() {
				if cerr := close(); err == nil {
					err = cerr
				}
			}()

			msg, ok := args.(proto.Message)
			if !ok {
				return nil, fmt.Errorf("expected proto message")
			}

			reqpb, err := starlarkproto.NewMessage(msg.ProtoReflect(), nil, nil)
			if err != nil {
				return nil, err
			}

			v, err := starlark.Call(thread, fn, starlark.Tuple{reqpb}, nil)
			if err != nil {
				return nil, err
			}

			rsppb, ok := v.(*starlarkproto.Message)
			if !ok {
				return nil, fmt.Errorf("expected \"proto.message\" received %q", v.Type())
			}
			rspMsg := rsppb.ProtoReflect()
			// Compare FullName for multiple descriptor implementations.
			if got, want := rspMsg.Descriptor().FullName(), replyDesc.FullName(); got != want {
				return nil, fmt.Errorf("invalid response type %s, want %s", got, want)
			}
			return rspMsg.Interface(), nil
		}
		h := func(opts *muxOptions, stream grpc.ServerStream) error {
			ctx := stream.Context()
			args := dynamicpb.NewMessage(argsDesc)

			if err := stream.RecvMsg(args); err != nil {
				return err
			}

			reply, err := opts.unary(ctx, args, info, fn)
			if err != nil {
				return err
			}
			return stream.SendMsg(reply)
		}

		return &handler{
			method:     method,
			descriptor: md,
			handler:    h,
		}
	}
}

func (m *Mux) registerStarlarkService(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {

	var name string
	if err := starlark.UnpackPositionalArgs(fnname, args, nil, 1, &name); err != nil {
		return nil, err
	}

	resolver := starlarkproto.GetProtodescResolver(thread)
	desc, err := resolver.FindDescriptorByName(protoreflect.FullName(name))
	if err != nil {
		return nil, err
	}

	sd, ok := desc.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "%q must be a service descriptor", name)
	}

	mds := sd.Methods()

	// for each key, assign the service function.

	mms := make(map[string]starlark.Callable)

	for _, kwarg := range kwargs {
		k := string(kwarg[0].(starlark.String))
		v := kwarg[1]

		// Check
		c, ok := v.(starlark.Callable)
		if !ok {
			return nil, status.Errorf(codes.InvalidArgument, "%s must be callable", k)
		}
		mms[k] = c
	}

	// Load the state for writing.
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.loadState().clone()

	for i, n := 0, mds.Len(); i < n; i++ {
		md := mds.Get(i)
		methodName := string(md.Name())

		c, ok := mms[methodName]
		if !ok {
			c = starext.MakeMethod(m, methodName, starlarkUnimplemented)
		}

		opts := md.Options()

		rule := getExtensionHTTP(opts)
		if rule == nil {
			continue
		}
		hd := createStarlarkHandler(thread, c, sd, md)
		if err := s.appendHandler(rule, md, hd); err != nil {
			return nil, err
		}
	}

	m.storeState(s)
	return starlark.None, nil
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
		return nil, nil // swallow error, reports missing attr.
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
	ctx, _ := newIncomingContext(s.ctx, nil) // TODO: remove me?
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
	if a := starlarkStreamAttrs[name]; a != nil {
		return a(s), nil
	}
	return nil, nil
}
func (v *StarlarkStream) AttrNames() []string {
	names := make([]string, 0, len(starlarkStreamAttrs))
	for name := range starlarkStreamAttrs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

type starlarkStreamAttr func(*StarlarkStream) starlark.Value

var starlarkStreamAttrs = map[string]starlarkStreamAttr{
	"recv": func(s *StarlarkStream) starlark.Value {
		return starext.MakeMethod(s, "recv", s.recv)
	},
	"send": func(s *StarlarkStream) starlark.Value {
		return starext.MakeMethod(s, "send", s.send)
	},
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

func (s *StarlarkStream) recv(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := starlarkthread.Context(thread)
	if err := s.init(thread); err != nil {
		return nil, err
	}

	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 0); err != nil {
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
func (s *StarlarkStream) send(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := starlarkthread.Context(thread)
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
