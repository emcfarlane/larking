// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package larking

import (
	"context"
	"fmt"
	"reflect"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type handlerFunc func(*muxOptions, grpc.ServerStream) error

type handler struct {
	method     string
	descriptor protoreflect.MethodDescriptor
	handler    handlerFunc
}

func (m *Mux) RegisterServiceByName(name protoreflect.FullName, srv interface{}) error {
	desc, err := m.opts.files.FindDescriptorByName(name)
	if err != nil {
		return err
	}
	sd, ok := desc.(protoreflect.ServiceDescriptor)
	if !ok {
		return fmt.Errorf("not a service descriptor %T", desc)
	}
	return m.RegisterService(sd, srv)
}

// TODO: move to server
//func (s *Server) RegisterService(desc *grpc.ServiceDesc, impl interface{}) {
func (m *Mux) RegisterService(sd protoreflect.ServiceDescriptor, srv interface{}) error {
	// Load the state for writing.
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.loadState().clone()

	types := m.opts.types
	name := sd.FullName()

	mds := sd.Methods()
	for j := 0; j < mds.Len(); j++ {
		// TODO: streaming
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

		h := func(ctx context.Context, req interface{}) (interface{}, error) {
			out := rm.Call([]reflect.Value{
				reflect.ValueOf(ctx), reflect.ValueOf(req),
			})
			errVal := out[1]
			if errVal.IsNil() {
				return out[0].Interface(), nil
			}
			return nil, out[1].Interface().(error)
		}

		methodName := fmt.Sprintf("/%s/%s", name, md.Name())
		info := &grpc.UnaryServerInfo{
			Server:     nil,
			FullMethod: methodName,
		}

		if err := s.appendHandler(rule, md, methodName, &handler{
			method:     methodName,
			descriptor: md,
			handler: func(opts *muxOptions, stream grpc.ServerStream) error {
				ctx := stream.Context()
				args := inProtoType.New().Interface()

				if err := stream.RecvMsg(args); err != nil {
					return err
				}

				reply, err := opts.unary(ctx, args, info, h)
				if err != nil {
					return err
				}

				return stream.SendMsg(reply)
			},
		}); err != nil {
			return err
		}
	}

	m.storeState(s)

	return nil
}

// serverTransportStream captures server metadata in memory.
type serverTransportStream struct {
	method     string
	sentHeader bool
	header     metadata.MD
	trailer    metadata.MD
}

func (s *serverTransportStream) Method() string { return s.method }
func (s *serverTransportStream) SetHeader(md metadata.MD) error {
	if !s.sentHeader {
		s.header = metadata.Join(s.header, md)
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
	s.trailer = metadata.Join(s.trailer, md)
	return nil
}
