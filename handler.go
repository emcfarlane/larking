// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package larking

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

type handlerMethod struct {
	inType protoreflect.MessageType
	unary  grpc.UnaryHandler
	//stream grpc.StreamHandler TODO: streaming
}

// Handler implements a http handler that wraps a gprc service implementation
// for google.http bindings.
type Handler struct {
	path    *path
	methods map[string]handlerMethod

	// Files to lookup service descriptors.
	// If nil, protoregistry.GlobalFiles is used.
	Files *protoregistry.Files

	// Types to create proto.Message.
	// If nil, protoregistry.GlobalTypes is used.
	Types protoregistry.MessageTypeResolver

	// UnaryInterceptor.
	UnaryInterceptor grpc.UnaryServerInterceptor

	// StreamInterceptor TODO: support streaming.
	StreamInterceptor grpc.StreamServerInterceptor
}

func (h *Handler) RegisterServiceByName(name protoreflect.FullName, srv interface{}) error {
	f := h.Files
	if f == nil {
		f = protoregistry.GlobalFiles
	}
	desc, err := f.FindDescriptorByName(name)
	if err != nil {
		return err
	}
	sd, ok := desc.(protoreflect.ServiceDescriptor)
	if !ok {
		return fmt.Errorf("not a service descriptor %T", desc)
	}
	return h.RegisterService(sd, srv)
}

func (h *Handler) RegisterService(sd protoreflect.ServiceDescriptor, srv interface{}) error {
	types := h.Types
	if types == nil {
		types = protoregistry.GlobalTypes
	}
	name := sd.FullName()

	mds := sd.Methods()
	for j := 0; j < mds.Len(); j++ {
		md := mds.Get(j)

		opts := md.Options() // TODO: nil check fails?

		rule := getExtensionHTTP(opts)
		if rule == nil {
			continue
		}

		rv := reflect.ValueOf(srv)
		rm := rv.MethodByName(string(md.Name()))
		if !rm.IsValid() {
			return fmt.Errorf("%T missing %s method", srv, md.Name())
		}

		inDesc := md.Input()
		outDesc := md.Output()

		inProtoType, err := types.FindMessageByName(inDesc.FullName())
		if err != nil {
			return err
		}
		outProtoType, err := types.FindMessageByName(outDesc.FullName())
		if err != nil {
			return err
		}
		in, out := inProtoType.Zero().Interface(), outProtoType.Zero().Interface()

		rmt := rm.Type()
		if rmt.NumIn() != 2 || rmt.NumOut() != 2 {
			return fmt.Errorf("invalid method %v", rmt)
		}

		ctxType := reflect.TypeOf((*context.Context)(nil)).Elem()
		if !rmt.In(0).Implements(ctxType) {
			return fmt.Errorf("invalid context type %v", rmt)
		}
		if rmt.In(1) != reflect.TypeOf(in) {
			return fmt.Errorf("invalid input type %v", rmt)
		}

		if rmt.Out(0) != reflect.TypeOf(out) {
			return fmt.Errorf("invalid output type %v", rmt)
		}
		errType := reflect.TypeOf((*error)(nil)).Elem()
		if !rmt.Out(1).Implements(errType) {
			return fmt.Errorf("invalid error type %v", rmt)
		}

		// init
		if h.path == nil {
			h.path = newPath()
			h.methods = make(map[string]handlerMethod)
		}

		methodName := fmt.Sprintf("/%s/%s", name, md.Name())
		if err := h.path.addRule(rule, md, methodName); err != nil {
			return err
		}

		h.methods[methodName] = handlerMethod{
			inType: inProtoType,
			unary: func(ctx context.Context, in interface{}) (interface{}, error) {
				out := rm.Call([]reflect.Value{
					reflect.ValueOf(ctx), reflect.ValueOf(in),
				})
				errVal := out[1]
				if errVal.IsNil() {
					return out[0].Interface(), nil
				}
				return nil, out[1].Interface().(error)
			},
		}
	}
	return nil
}

// serverTransportStream captures server metadata in memory.
type serverTransportStream struct {
	method     string
	sentHeader bool
	header     metadata.MD
	trailer    metadata.MD
}

func (s *serverTransportStream) Method() string {
	return s.method
}
func (s *serverTransportStream) SetHeader(md metadata.MD) error {
	if !s.sentHeader {
		s.header = md
	}
	return nil

}
func (s *serverTransportStream) SendHeader(md metadata.MD) error {
	if err := s.SetHeader(md); err != nil {
		return err
	}
	s.sentHeader = true
	return nil
}
func (s *serverTransportStream) SetTrailer(md metadata.MD) error {
	s.sentHeader = true
	s.trailer = md
	return nil
}

func (h *Handler) serveHTTP(w http.ResponseWriter, r *http.Request) error {
	if !strings.HasPrefix(r.URL.Path, "/") {
		r.URL.Path = "/" + r.URL.Path
	}

	method, params, err := h.path.match(r.URL.Path, r.Method)
	if err != nil {
		return err
	}
	mh := h.methods[method.name]

	args := mh.inType.New().Interface()

	if method.hasBody {
		if err := method.decodeRequestArgs(args, r); err != nil {
			return err
		}
	}

	queryParams, err := method.parseQueryParams(r.URL.Query())
	if err != nil {
		return err
	}
	params = append(params, queryParams...)
	if err := params.set(args); err != nil {
		return err
	}

	ctx := newIncomingContext(r.Context(), r.Header)
	stream := &serverTransportStream{
		method: method.name,
	}
	ctx = grpc.NewContextWithServerTransportStream(ctx, stream)

	// TODO: support streaming
	var replyI interface{}
	if h.UnaryInterceptor != nil {
		info := &grpc.UnaryServerInfo{
			FullMethod: method.name,
		}
		replyI, err = h.UnaryInterceptor(ctx, args, info, mh.unary)
	} else {
		replyI, err = mh.unary(ctx, args)
	}
	if err != nil {
		setOutgoingHeader(w.Header(), stream.header, stream.trailer)
		return err
	}
	reply := replyI.(proto.Message)

	return method.encodeResponseReply(reply, w, r, stream.header, stream.trailer)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h.serveHTTP(w, r); err != nil {
		encError(w, err)
	}
}
