// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package larking

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/genproto/googleapis/api/httpbody"
	"google.golang.org/genproto/googleapis/api/serviceconfig"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	rpb "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
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

// https://cloud.google.com/service-infrastructure/docs/service-management/reference/rpc/google.api#google.api.DocumentationRule.FIELDS.string.google.api.DocumentationRule.selector
// The selector is a comma-separated list of patterns for any element such as a
// method, a field, an enum value. Each pattern is a qualified name of the
// element which may end in "*", indicating a wildcard. Wildcards are only
// allowed at the end and for a whole component of the qualified name, i.e.
// "foo.*" is ok, but not "foo.b*" or "foo.*.bar". A wildcard will match one
// or more components. To specify a default for all applicable elements, the
// whole pattern "*" is used.
type ruleSelector struct {
	path  map[string]*ruleSelector
	rules []*annotations.HttpRule
}

func (r *ruleSelector) write(w io.Writer, indent string) {
	for key, rs := range r.path {
		fmt.Fprintf(w, "%s%s: \n", indent, key)
		rs.write(w, indent+"  ")
	}
	fmt.Fprintf(w, "%srules: %v\n", indent, r.rules)
}

// String returns the string representation of the ruleSelector.
func (r *ruleSelector) String() string {
	buf := strings.Builder{}
	r.write(&buf, "")
	return buf.String()
}

func (r *ruleSelector) getRules(name string) (rules []*annotations.HttpRule) {
	rules = append(rules, r.rules...)
	if name == "" {
		return rules
	}
	tag, name, _ := strings.Cut(name, ".")
	if r = r.path[tag]; r != nil {
		return append(rules, r.getRules(name)...)
	}
	return rules
}

func (r *ruleSelector) setRules(rules []*annotations.HttpRule) {
	*r = ruleSelector{} // reset

	var set func(r *ruleSelector, selector string)
	for _, rule := range rules {
		set = func(r *ruleSelector, selector string) {
			tag, name, _ := strings.Cut(selector, ".")
			switch tag {
			case "*":
				if name != "" {
					panic(fmt.Errorf("invalid selector %q", rule.GetSelector()))
				}
				r.rules = append(r.rules, rule)
			case "":
				r.rules = append(r.rules, rule)
			default:
				rs := r.path[tag]
				if rs == nil {
					rs = &ruleSelector{}
				}
				if r.path == nil {
					r.path = make(map[string]*ruleSelector)
				}
				r.path[tag] = rs
				r = rs
				set(r, name)
			}
		}

		set(r, rule.GetSelector())
	}
}

type muxOptions struct {
	types                 protoregistry.MessageTypeResolver
	statsHandler          stats.Handler
	files                 *protoregistry.Files
	serviceConfig         *serviceconfig.Service
	unaryInterceptor      grpc.UnaryServerInterceptor
	streamInterceptor     grpc.StreamServerInterceptor
	codecs                map[string]Codec
	codecsByName          map[string]Codec
	compressors           map[string]Compressor
	httprules             ruleSelector
	contentTypeOffers     []string
	encodingTypeOffers    []string
	maxReceiveMessageSize int
	maxSendMessageSize    int
	connectionTimeout     time.Duration
}

// readAll reads from r until an error or EOF and returns the data it read.
func (o *muxOptions) readAll(b []byte, r io.Reader) ([]byte, error) {
	var total int64
	for {
		if len(b) == cap(b) {
			// Add more capacity (let append pick how much).
			b = append(b, 0)[:len(b)]
		}
		n, err := r.Read(b[len(b):cap(b)])
		b = b[:len(b)+n]
		total += int64(n)
		if total > int64(o.maxReceiveMessageSize) {
			return nil, fmt.Errorf("max receive message size reached")
		}
		if err != nil {
			return b, err
		}
	}
}
func (o *muxOptions) writeAll(dst io.Writer, b []byte) error {
	if len(b) > o.maxSendMessageSize {
		return fmt.Errorf("max send message size reached")
	}
	n, err := dst.Write(b)
	if err == nil && n != len(b) {
		return io.ErrShortWrite
	}
	return err
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

// MuxOption is an option for a mux.
type MuxOption func(*muxOptions)

const (
	defaultServerMaxReceiveMessageSize = 1024 * 1024 * 4
	defaultServerMaxSendMessageSize    = math.MaxInt32
	defaultServerConnectionTimeout     = 120 * time.Second
)

var (
	defaultMuxOptions = muxOptions{
		maxReceiveMessageSize: defaultServerMaxReceiveMessageSize,
		maxSendMessageSize:    defaultServerMaxSendMessageSize,
		connectionTimeout:     defaultServerConnectionTimeout,
		files:                 protoregistry.GlobalFiles,
		types:                 protoregistry.GlobalTypes,
	}

	defaultCodecs = map[string]Codec{
		"application/json":         CodecJSON{},
		"application/protobuf":     CodecProto{},
		"application/octet-stream": CodecProto{},
		"google.api.HttpBody":      codecHTTPBody{},
	}

	defaultCompressors = map[string]Compressor{
		"gzip":     &CompressorGzip{},
		"identity": nil,
	}
)

func UnaryServerInterceptorOption(interceptor grpc.UnaryServerInterceptor) MuxOption {
	return func(opts *muxOptions) { opts.unaryInterceptor = interceptor }
}

func StreamServerInterceptorOption(interceptor grpc.StreamServerInterceptor) MuxOption {
	return func(opts *muxOptions) { opts.streamInterceptor = interceptor }
}

func StatsOption(h stats.Handler) MuxOption {
	return func(opts *muxOptions) { opts.statsHandler = h }
}

func MaxReceiveMessageSizeOption(s int) MuxOption {
	return func(opts *muxOptions) { opts.maxReceiveMessageSize = s }
}
func MaxSendMessageSizeOption(s int) MuxOption {
	return func(opts *muxOptions) { opts.maxSendMessageSize = s }
}
func ConnectionTimeoutOption(d time.Duration) MuxOption {
	return func(opts *muxOptions) { opts.connectionTimeout = d }
}
func TypesOption(t protoregistry.MessageTypeResolver) MuxOption {
	return func(opts *muxOptions) { opts.types = t }
}
func FilesOption(f *protoregistry.Files) MuxOption {
	return func(opts *muxOptions) { opts.files = f }
}

// CodecOption registers a codec for the given content type.
func CodecOption(contentType string, c Codec) MuxOption {
	return func(opts *muxOptions) {
		if opts.codecs == nil {
			opts.codecs = make(map[string]Codec)
		}
		opts.codecs[contentType] = c
	}
}

// CompressorOption registers a compressor for the given content encoding.
func CompressorOption(contentEncoding string, c Compressor) MuxOption {
	return func(opts *muxOptions) {
		if opts.compressors == nil {
			opts.compressors = make(map[string]Compressor)
		}
		opts.compressors[contentEncoding] = c
	}
}

// ServiceConfigOption sets the service config for the mux.
// Currently only  http rules will be used to annotate services.
func ServiceConfigOption(sc *serviceconfig.Service) MuxOption {
	return func(opts *muxOptions) {
		opts.serviceConfig = sc
		opts.httprules = ruleSelector{}
		opts.httprules.setRules(sc.Http.GetRules())

	}
}

type Mux struct {
	opts  muxOptions
	state atomic.Value
	mu    sync.Mutex
}

func NewMux(opts ...MuxOption) (*Mux, error) {
	// Apply options.
	var muxOpts = defaultMuxOptions
	for _, opt := range opts {
		opt(&muxOpts)
	}

	// Ensure codecs are set.
	if muxOpts.codecs == nil {
		muxOpts.codecs = make(map[string]Codec)
	}
	for k, v := range defaultCodecs {
		if _, ok := muxOpts.codecs[k]; !ok {
			muxOpts.codecs[k] = v
		}
	}
	muxOpts.codecsByName = make(map[string]Codec)
	for _, v := range muxOpts.codecs {
		muxOpts.codecsByName[v.Name()] = v
	}
	for k := range muxOpts.codecs {
		muxOpts.contentTypeOffers = append(muxOpts.contentTypeOffers, k)
	}
	sort.Strings(muxOpts.contentTypeOffers)

	// Ensure compressors are set.
	if muxOpts.compressors == nil {
		muxOpts.compressors = make(map[string]Compressor)
	}
	for k, v := range defaultCompressors {
		if _, ok := muxOpts.compressors[k]; !ok {
			muxOpts.compressors[k] = v
		}
	}
	for k := range muxOpts.codecs {
		muxOpts.encodingTypeOffers = append(muxOpts.encodingTypeOffers, k)
	}
	sort.Strings(muxOpts.encodingTypeOffers)

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

	if err := s.addConnHandler(m.opts, cc, stream); err != nil {
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
	stream rpb.ServerReflection_ServerReflectionInfoClient
	files  protoregistry.Files
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
	opts muxOptions,
	desc protoreflect.MethodDescriptor,
	h *handler,
) error {
	// Add an implicit rule for the method.
	implicitRule := &annotations.HttpRule{
		Pattern: &annotations.HttpRule_Custom{
			Custom: &annotations.CustomHttpPattern{
				Kind: "*",
				Path: h.method,
			},
		},
		Body: "*",
	}
	if err := s.path.addRule(implicitRule, desc, h.method); err != nil {
		panic(fmt.Sprintf("bug: %v", err))
	}

	// Add all ServiceConfig.http rules.
	name := string(desc.FullName())
	for _, rule := range opts.httprules.getRules(name) {
		if err := s.path.addRule(rule, desc, h.method); err != nil {
			return fmt.Errorf("[%s] invalid ServiceConfig.http rule %s: %w", desc.FullName(), rule.String(), err)
		}
	}

	// Add all annotated rules.
	if rule := getExtensionHTTP(desc.Options()); rule != nil {
		if err := s.path.addRule(rule, desc, h.method); err != nil {
			return fmt.Errorf("[%s] invalid rule %s: %w", desc.FullName(), rule.String(), err)
		}
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
	opts muxOptions,
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

		hs, err := s.processFile(opts, cc, file)
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

func createConnHandler(
	cc *grpc.ClientConn,
	sd protoreflect.ServiceDescriptor,
	md protoreflect.MethodDescriptor,
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
						args := dynamicpb.NewMessage(argsDesc)
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
				reply := dynamicpb.NewMessage(replyDesc)
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
			method:  method,
			desc:    md,
			handler: h,
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
			method:  method,
			desc:    md,
			handler: h,
		}
	}
}

func (s *state) processFile(opts muxOptions, cc *grpc.ClientConn, fd protoreflect.FileDescriptor) ([]*handler, error) {
	var handlers []*handler

	sds := fd.Services()
	for i := 0; i < sds.Len(); i++ {
		sd := sds.Get(i)

		mds := sd.Methods()
		for j := 0; j < mds.Len(); j++ {
			md := mds.Get(j)
			hd := createConnHandler(cc, sd, md)
			if err := s.appendHandler(opts, md, hd); err != nil {
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

// ServeHTTP implements http.Handler.
// It supports both gRPC and HTTP requests.
func (m *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.ProtoMajor == 2 && strings.HasPrefix(
		r.Header.Get("Content-Type"), "application/grpc",
	) {
		m.serveGRPC(w, r)
		return
	}

	if strings.HasPrefix(
		r.Header.Get("Content-Type"), "application/grpc-web",
	) {
		m.serveGRPCWeb(w, r)
		return
	}

	if !strings.HasPrefix(r.URL.Path, "/") {
		r.URL.Path = "/" + r.URL.Path
	}
	r.URL.Path = strings.TrimSuffix(r.URL.Path, "/")
	if err := m.serveHTTP(w, r); err != nil {
		m.encError(w, r, err)
	}
}
