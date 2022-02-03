// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package larking

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/genproto/googleapis/api/httpbody"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	rpb "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
	"nhooyr.io/websocket"
)

// RO
type connList struct {
	handlers []*handler
	fdHash   []byte
}

type state struct {
	path     *path
	conns    map[*grpc.ClientConn]connList
	handlers map[string][]*handler
}

func (s *state) clone() *state {
	if s == nil {
		return &state{
			path:     newPath(),
			conns:    make(map[*grpc.ClientConn]connList),
			handlers: make(map[string][]*handler),
		}
	}

	conns := make(map[*grpc.ClientConn]connList)
	for conn, cl := range s.conns {
		conns[conn] = cl
	}

	handlers := make(map[string][]*handler)
	for method, hds := range s.handlers {
		handlers[method] = hds
	}

	return &state{
		path:     s.path.clone(),
		conns:    conns,
		handlers: handlers,
	}
}

type muxOptions struct {
	maxReceiveMessageSize int
	maxSendMessageSize    int
	connectionTimeout     time.Duration
	files                 *protoregistry.Files
	types                 protoregistry.MessageTypeResolver
	unaryInterceptor      grpc.UnaryServerInterceptor
	streamInterceptor     grpc.StreamServerInterceptor
}

// unary is a nil-safe interceptor unary call.
func (o *muxOptions) unary(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
	if ui := o.unaryInterceptor; ui != nil {
		return ui(ctx, req, info, handler)
	}
	return handler(ctx, req)
}

// stream is a nil-safe interceptor stream call.
func (o *muxOptions) stream(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	if si := o.streamInterceptor; si != nil {
		return si(srv, ss, info, handler)
	}
	return handler(srv, ss)
}

type MuxOption func(*muxOptions)

var defaultMuxOptions = muxOptions{
	maxReceiveMessageSize: defaultServerMaxReceiveMessageSize,
	maxSendMessageSize:    defaultServerMaxSendMessageSize,
	connectionTimeout:     defaultServerConnectionTimeout,
	files:                 protoregistry.GlobalFiles,
	types:                 protoregistry.GlobalTypes,
}

func UnaryServerInterceptorOption(interceptor grpc.UnaryServerInterceptor) MuxOption {
	return func(opts *muxOptions) { opts.unaryInterceptor = interceptor }
}

func StreamServerInterceptorOption(interceptor grpc.StreamServerInterceptor) MuxOption {
	return func(opts *muxOptions) { opts.streamInterceptor = interceptor }
}

//// LogOption adds a logr with a unary and stream contexts.
//func LogOption(log logr.Logger) MuxOption {
//	return func(opts *muxOption) { opts.log = log }
//}

type Mux struct {
	opts muxOptions
	//events trace.EventLog TODO
	mu    sync.Mutex   // Lock to sync writers
	state atomic.Value // Value of *state

	// services is a list of registered services
	services map[*grpc.ServiceDesc]interface{}
}

func NewMux(opts ...MuxOption) (*Mux, error) {
	// Apply options.
	var muxOpts = defaultMuxOptions
	for _, opt := range opts {
		opt(&muxOpts)
	}

	return &Mux{
		opts: muxOpts,
	}, nil
}

func (m *Mux) RegisterConn(ctx context.Context, cc *grpc.ClientConn) error {
	c := rpb.NewServerReflectionClient(cc)

	// TODO: watch the stream. When it is recreated refresh the service
	// methods and recreate the mux if needed.
	stream, err := c.ServerReflectionInfo(ctx, grpc.WaitForReady(true))
	if err != nil {
		return err
	}

	// Load the state for writing.
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.loadState().clone()

	if err := s.addConnHandler(cc, stream); err != nil {
		return err
	}

	m.storeState(s)

	return stream.CloseSend()
}

func (m *Mux) DropConn(ctx context.Context, cc *grpc.ClientConn) bool {
	// Load the state for writing.
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.loadState().clone()

	return s.removeHandler(cc)
}

// resolver implements protodesc.Resolver.
type resolver struct {
	files  protoregistry.Files
	stream rpb.ServerReflection_ServerReflectionInfoClient
}

func newResolver(stream rpb.ServerReflection_ServerReflectionInfoClient) (*resolver, error) {
	r := &resolver{stream: stream}

	if err := r.files.RegisterFile(annotations.File_google_api_annotations_proto); err != nil {
		return nil, err
	}
	if err := r.files.RegisterFile(annotations.File_google_api_http_proto); err != nil {
		return nil, err
	}
	if err := r.files.RegisterFile(httpbody.File_google_api_httpbody_proto); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *resolver) FindFileByPath(path string) (protoreflect.FileDescriptor, error) {
	if fd, err := r.files.FindFileByPath(path); err == nil {
		return fd, nil // found file
	}

	if err := r.stream.Send(&rpb.ServerReflectionRequest{
		MessageRequest: &rpb.ServerReflectionRequest_FileByFilename{
			FileByFilename: path,
		},
	}); err != nil {
		return nil, err
	}

	fdr, err := r.stream.Recv()
	if err != nil {
		return nil, err
	}
	fdbs := fdr.GetFileDescriptorResponse().GetFileDescriptorProto()

	var f protoreflect.FileDescriptor
	for _, fdb := range fdbs {
		fdp := &descriptorpb.FileDescriptorProto{}
		if err := proto.Unmarshal(fdb, fdp); err != nil {
			return nil, err
		}

		file, err := protodesc.NewFile(fdp, r)
		if err != nil {
			return nil, err
		}
		// TODO: check duplicate file registry
		if err := r.files.RegisterFile(file); err != nil {
			return nil, err
		}
		if file.Path() == path {
			f = file
		}
	}
	if f == nil {
		return nil, fmt.Errorf("missing file descriptor %s", path)
	}
	return f, nil
}

func (r *resolver) FindDescriptorByName(fullname protoreflect.FullName) (protoreflect.Descriptor, error) {
	return r.files.FindDescriptorByName(fullname)
}

func (s *state) appendHandler(
	rule *annotations.HttpRule,
	desc protoreflect.MethodDescriptor,
	h *handler,
) error {
	if err := s.path.addRule(rule, desc, h.method); err != nil {
		return err
	}
	s.handlers[h.method] = append(s.handlers[h.method], h)
	return nil
}

func (s *state) removeHandler(cc *grpc.ClientConn) bool {
	cl, ok := s.conns[cc]
	if !ok {
		return ok
	}

	// Drop handlers belonging to the client conn.
	for _, hd := range cl.handlers {
		name := hd.method

		var hds []*handler
		for _, mhd := range s.handlers[name] {
			// Compare if handler belongs to this connection.
			if mhd != hd {
				hds = append(hds, mhd)
			}
		}
		if len(hds) == 0 {
			delete(s.handlers, name)
			s.path.delRule(name)
		} else {
			s.handlers[name] = hds
		}
	}
	// Drop conn on client conn.
	delete(s.conns, cc)
	return ok
}

func (s *state) addConnHandler(
	cc *grpc.ClientConn,
	stream rpb.ServerReflection_ServerReflectionInfoClient,
) error {
	// TODO: async fetch and mux creation.

	if err := stream.Send(&rpb.ServerReflectionRequest{
		MessageRequest: &rpb.ServerReflectionRequest_ListServices{},
	}); err != nil {
		return err
	}

	r, err := stream.Recv()
	if err != nil {
		return err
	}
	// TODO: check r.GetErrorResponse()?

	// File descriptors hash for detecting updates. TODO: sort fds?
	h := sha256.New()

	fds := make(map[string]*descriptorpb.FileDescriptorProto)
	for _, svc := range r.GetListServicesResponse().GetService() {
		if err := stream.Send(&rpb.ServerReflectionRequest{
			MessageRequest: &rpb.ServerReflectionRequest_FileContainingSymbol{
				FileContainingSymbol: svc.GetName(),
			},
		}); err != nil {
			return err
		}

		fdr, err := stream.Recv()
		if err != nil {
			return err
		}

		fdbb := fdr.GetFileDescriptorResponse().GetFileDescriptorProto()

		for _, fdb := range fdbb {
			fd := &descriptorpb.FileDescriptorProto{}
			if err := proto.Unmarshal(fdb, fd); err != nil {
				return err
			}
			fds[fd.GetName()] = fd

			if _, err := h.Write(fdb); err != nil {
				return err
			}
		}
	}

	fdHash := h.Sum(nil)

	// Check if previous connection exists.
	if cl, ok := s.conns[cc]; ok {
		if bytes.Equal(cl.fdHash, fdHash) {
			return nil // nothing to do
		}

		// Drop and recreate below.
		s.removeHandler(cc)
	}

	rslvr, err := newResolver(stream)
	if err != nil {
		return err
	}

	var handlers []*handler
	for _, fd := range fds {
		file, err := protodesc.NewFile(fd, rslvr)
		if err != nil {
			return err
		}

		hs, err := s.processFile(cc, file)
		if err != nil {
			return err
		}
		handlers = append(handlers, hs...)
	}

	// Update methods list.
	s.conns[cc] = connList{
		handlers: handlers,
		fdHash:   fdHash,
	}
	return nil
}

func (s *state) createConnHandler(
	cc *grpc.ClientConn,
	sd protoreflect.ServiceDescriptor,
	md protoreflect.MethodDescriptor,
	rule *annotations.HttpRule,
) *handler {

	argsDesc := md.Input()
	replyDesc := md.Output()

	method := fmt.Sprintf("/%s/%s", sd.FullName(), md.Name())

	isClientStream := md.IsStreamingClient()
	isServerStream := md.IsStreamingServer()
	if isClientStream || isServerStream {
		sd := &grpc.StreamDesc{
			ServerStreams: md.IsStreamingServer(),
			ClientStreams: md.IsStreamingClient(),
		}
		info := &grpc.StreamServerInfo{
			FullMethod:     method,
			IsClientStream: isClientStream,
			IsServerStream: isServerStream,
		}

		fn := func(_ interface{}, stream grpc.ServerStream) error {
			ctx := stream.Context()

			args := dynamicpb.NewMessage(argsDesc)
			reply := dynamicpb.NewMessage(replyDesc)

			if err := stream.RecvMsg(args); err != nil {
				return err
			}

			if md, ok := metadata.FromIncomingContext(ctx); ok {
				ctx = metadata.NewOutgoingContext(ctx, md)
			}

			clientStream, err := cc.NewStream(ctx, sd, method)
			if err != nil {
				return err
			}
			if err := clientStream.SendMsg(args); err != nil {
				return err
			}

			var inErr error
			var wg sync.WaitGroup
			if sd.ClientStreams {
				wg.Add(1)
				go func() {
					for {
						if inErr = stream.RecvMsg(args); inErr != nil {
							break
						}

						if inErr = clientStream.SendMsg(args); inErr != nil {
							break
						}
					}
					wg.Done()
				}()
			}
			var outErr error
			for {
				if outErr = clientStream.RecvMsg(reply); outErr != nil {
					break
				}

				if outErr = stream.SendMsg(reply); outErr != nil {
					break
				}

				if !sd.ServerStreams {
					break
				}
			}

			if isStreamError(outErr) {
				return outErr
			}
			if sd.ClientStreams {
				wg.Wait()
				if isStreamError(inErr) {
					return inErr
				}
			}
			trailer := clientStream.Trailer()
			stream.SetTrailer(trailer)
			return nil
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
		fn := func(ctx context.Context, args interface{}) (interface{}, error) {
			reply := dynamicpb.NewMessage(replyDesc)

			if md, ok := metadata.FromIncomingContext(ctx); ok {
				ctx = metadata.NewOutgoingContext(ctx, md)
			}

			if err := cc.Invoke(ctx, method, args, reply); err != nil {
				return nil, err
			}
			return reply, nil
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

func (s *state) processFile(cc *grpc.ClientConn, fd protoreflect.FileDescriptor) ([]*handler, error) {
	var handlers []*handler

	sds := fd.Services()
	for i := 0; i < sds.Len(); i++ {
		sd := sds.Get(i)

		mds := sd.Methods()
		for j := 0; j < mds.Len(); j++ {
			md := mds.Get(j)

			opts := md.Options() // TODO: nil check fails?

			rule := getExtensionHTTP(opts)
			if rule == nil {
				continue
			}

			hd := s.createConnHandler(cc, sd, md, rule)

			if err := s.appendHandler(rule, md, hd); err != nil {
				return nil, err
			}
			handlers = append(handlers, hd)
		}
	}
	return handlers, nil
}

func (m *Mux) loadState() *state {
	s, _ := m.state.Load().(*state)
	return s
}
func (m *Mux) storeState(s *state) { m.state.Store(s) }

func (s *state) pickMethodHandler(name string) (*handler, error) {
	if s != nil {
		hds := s.handlers[name]
		if len(hds) > 0 {
			hd := hds[rand.Intn(len(hds))]
			return hd, nil
		}
	}
	return nil, status.Errorf(codes.Unimplemented, "method %s not implemented", name)
}

func (s *state) match(route, verb string) (*method, params, error) {
	if s == nil {
		return nil, nil, status.Error(codes.NotFound, "not found")
	}
	return s.path.match(route, verb)
}

type streamHTTP struct {
	ctx    context.Context
	w      http.ResponseWriter
	r      *http.Request
	method *method
	params params
	recvN  int
	sendN  int

	sentHeader bool
	header     metadata.MD
	trailer    metadata.MD
}

func (s *streamHTTP) SetHeader(md metadata.MD) error {
	if !s.sentHeader {
		s.header = metadata.Join(s.header, md)
	}
	return nil

}
func (s *streamHTTP) SendHeader(md metadata.MD) error {
	if s.sentHeader {
		return nil // already sent?
	}
	setOutgoingHeader(s.w.Header(), s.header, s.trailer)
	s.w.WriteHeader(http.StatusOK)
	s.sentHeader = true
	return nil
}

func (s *streamHTTP) SetTrailer(md metadata.MD) {
	s.sentHeader = true
	s.trailer = metadata.Join(s.trailer, md)
}

func (s *streamHTTP) Context() context.Context {
	sts := &serverTransportStream{s, s.method.name}
	return grpc.NewContextWithServerTransportStream(s.ctx, sts)
}

func (s *streamHTTP) SendMsg(m interface{}) error {
	s.sendN += 1
	reply := m.(proto.Message)

	accept := s.r.Header.Get("Accept")
	acceptEncoding := s.r.Header.Get("Accept-Encoding")

	if fRsp, ok := s.w.(http.Flusher); ok {
		defer fRsp.Flush()
	}

	setOutgoingHeader(s.w.Header(), s.header, s.trailer)

	var resp io.Writer
	switch acceptEncoding {
	case "gzip":
		s.w.Header().Set("Content-Encoding", "gzip")
		gRsp := gzip.NewWriter(s.w)
		defer gRsp.Close()
		resp = gRsp

	default:
		resp = s.w
	}

	cur := reply.ProtoReflect()
	for _, fd := range s.method.resp {
		cur = cur.Mutable(fd).Message()
	}

	msg := cur.Interface()

	if cur.Descriptor().FullName() == "google.api.HttpBody" {
		fds := cur.Descriptor().Fields()
		fdContentType := fds.ByName(protoreflect.Name("content_type"))
		fdData := fds.ByName(protoreflect.Name("data"))
		pContentType := cur.Get(fdContentType)
		pData := cur.Get(fdData)

		s.w.Header().Set("Content-Type", pContentType.String())
		if _, err := io.Copy(resp, bytes.NewReader(pData.Bytes())); err != nil {
			return err
		}
		return nil
	}

	switch accept {
	case "application/protobuf", "application/octet-stream":
		b, err := proto.Marshal(msg)
		if err != nil {
			return err
		}
		s.w.Header().Set("Content-Type", accept)
		if _, err := io.Copy(resp, bytes.NewReader(b)); err != nil {
			return err
		}
		return nil

	default:
		b, err := protojson.Marshal(msg)
		if err != nil {
			return err
		}
		s.w.Header().Set("Content-Type", "application/json")
		if _, err := io.Copy(resp, bytes.NewReader(b)); err != nil {
			return err
		}
		return nil
	}
}

func (s *streamHTTP) decodeRequestArgs(args proto.Message) error {
	contentType := s.r.Header.Get("Content-Type")
	contentEncoding := s.r.Header.Get("Content-Encoding")

	var body io.ReadCloser
	switch contentEncoding {
	case "gzip":
		var err error
		body, err = gzip.NewReader(s.r.Body)
		if err != nil {
			return err
		}

	default:
		body = s.r.Body
	}
	defer body.Close()

	// TODO: mux options.
	b, err := ioutil.ReadAll(io.LimitReader(body, 1024*1024*2))
	if err != nil {
		return err
	}

	cur := args.ProtoReflect()
	for _, fd := range s.method.body {
		cur = cur.Mutable(fd).Message()
	}
	fullname := cur.Descriptor().FullName()

	msg := cur.Interface()

	if fullname == "google.api.HttpBody" {
		rfl := msg.ProtoReflect()
		fds := rfl.Descriptor().Fields()
		fdContentType := fds.ByName(protoreflect.Name("content_type"))
		fdData := fds.ByName(protoreflect.Name("data"))
		rfl.Set(fdContentType, protoreflect.ValueOfString(contentType))
		rfl.Set(fdData, protoreflect.ValueOfBytes(b))
		// TODO: extensions?
		return nil
	}

	switch contentType {
	case "application/protobuf", "application/octet-stream":
		if err := proto.Unmarshal(b, msg); err != nil {
			return err
		}

	default:
		// What marshalling options should we support?
		if err := protojson.Unmarshal(b, msg); err != nil {
			return err
		}
	}
	return nil
}

func (s *streamHTTP) RecvMsg(m interface{}) error {
	s.recvN += 1
	args := m.(proto.Message)

	// TODO: fix the body marshalling
	if s.method.hasBody {
		// TODO: handler should decide what to select on?
		if err := s.decodeRequestArgs(args); err != nil {
			return err
		}
	}
	if s.recvN == 1 {
		if err := s.params.set(args); err != nil {
			return err
		}
	}
	return nil
}

func isWebsocketRequest(r *http.Request) bool {
	for _, header := range r.Header["Upgrade"] {
		if header == "websocket" {
			return true
		}
	}
	return false
}

func encError(w http.ResponseWriter, err error) {
	s, _ := status.FromError(err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(HTTPStatusCode(s.Code()))

	b, err := protojson.Marshal(s.Proto())
	if err != nil {
		panic(err) // ...
	}
	w.Write(b) //nolint
}

func (m *Mux) serveHTTP(w http.ResponseWriter, r *http.Request) error {
	ctx := newIncomingContext(r.Context(), r.Header)

	// TOOD: debug flag?
	//d, err := httputil.DumpRequest(r, true)
	//if err != nil {
	//	return err
	//}

	s := m.loadState()
	isWebsocket := isWebsocketRequest(r)

	verb := r.Method
	if isWebsocket {
		verb = kindWebsocket
	}

	method, params, err := s.match(r.URL.Path, verb)
	if err != nil {
		return err
	}

	queryParams, err := method.parseQueryParams(r.URL.Query())
	if err != nil {
		return err
	}
	params = append(params, queryParams...)

	hd, err := s.pickMethodHandler(method.name)
	if err != nil {
		return err
	}

	if isWebsocket {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{})
		if err != nil {
			return err
		}

		stream := &streamWS{
			ctx:    ctx,
			conn:   c,
			method: method,
			params: params,
		}
		if err := hd.handler(&m.opts, stream); err != nil {
			s, _ := status.FromError(err)
			// TODO: limit message size.
			c.Close(WSStatusCode(s.Code()), s.Message()) // TODO
			return nil
		}
		c.Close(websocket.StatusNormalClosure, "OK") // TODO
		return nil
	}

	stream := &streamHTTP{
		ctx: ctx,
		w:   w, r: r,
		method: method,
		params: params,
	}
	return hd.handler(&m.opts, stream)
}

func (m *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/") {
		r.URL.Path = "/" + r.URL.Path
	}
	if err := m.serveHTTP(w, r); err != nil {
		encError(w, err)
	}
}
