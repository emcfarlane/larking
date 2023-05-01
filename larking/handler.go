// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package larking

import (
	"fmt"
	"log"
	"reflect"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type handlerFunc func(*muxOptions, grpc.ServerStream) error

type handler struct {
	descriptor protoreflect.MethodDescriptor
	handler    handlerFunc
	method     string // /Service/Method
}

// TODO: use grpclog?
//var logger = grpclog.Component("core")

// RegisterService satisfies grpc.ServiceRegistrar for generated service code hooks.
func (m *Mux) RegisterService(sd *grpc.ServiceDesc, ss interface{}) {
	if ss != nil {
		ht := reflect.TypeOf(sd.HandlerType).Elem()
		st := reflect.TypeOf(ss)
		if !st.Implements(ht) {
			log.Fatalf("larking: RegisterService found the handler of type %v that does not satisfy %v", st, ht)
		}
	}
	if err := m.registerService(sd, ss); err != nil {
		log.Fatalf("larking: RegisterService error: %v", err)
	}
}

func (m *Mux) registerService(gsd *grpc.ServiceDesc, ss interface{}) error {

	// Load the state for writing.
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.loadState().clone()

	d, err := m.opts.files.FindDescriptorByName(protoreflect.FullName(gsd.ServiceName))
	if err != nil {
		return err
	}
	sd, ok := d.(protoreflect.ServiceDescriptor)
	if !ok {
		return fmt.Errorf("invalid method descriptor %T", d)
	}
	mds := sd.Methods()

	findMethod := func(methodName string) (protoreflect.MethodDescriptor, error) {
		md := mds.ByName(protoreflect.Name(methodName))
		if md == nil {
			return nil, fmt.Errorf("missing method descriptor for %v", methodName)
		}
		return md, nil
	}

	for i := range gsd.Methods {
		d := &gsd.Methods[i]
		method := "/" + gsd.ServiceName + "/" + d.MethodName

		md, err := findMethod(d.MethodName)
		if err != nil {
			return err
		}

		h := &handler{
			method:     method,
			descriptor: md,
			handler: func(opts *muxOptions, stream grpc.ServerStream) error {
				ctx := stream.Context()

				// TODO: opts?
				reply, err := d.Handler(ss, ctx, stream.RecvMsg, opts.unaryInterceptor)
				if err != nil {
					return err
				}
				return stream.SendMsg(reply)
			},
		}

		if err := s.appendHandler(md, h); err != nil {
			return err
		}
	}
	for i := range gsd.Streams {
		d := &gsd.Streams[i]
		method := "/" + gsd.ServiceName + "/" + d.StreamName
		md, err := findMethod(d.StreamName)
		if err != nil {
			return err
		}

		h := &handler{
			method:     method,
			descriptor: md,
			handler: func(opts *muxOptions, stream grpc.ServerStream) error {
				info := &grpc.StreamServerInfo{
					FullMethod:     method,
					IsClientStream: d.ClientStreams,
					IsServerStream: d.ServerStreams,
				}

				return opts.stream(ss, stream, info, d.Handler)
			},
		}
		if err := s.appendHandler(md, h); err != nil {
			return err
		}
	}

	m.storeState(s)
	return nil
}

var _ grpc.ServerTransportStream = (*serverTransportStream)(nil)

// serverTransportStream wraps gprc.SeverStream to support header/trailers.
type serverTransportStream struct {
	grpc.ServerStream
	method string
}

func (s *serverTransportStream) Method() string { return s.method }
func (s *serverTransportStream) SetHeader(md metadata.MD) error {
	return s.ServerStream.SetHeader(md)
}
func (s *serverTransportStream) SendHeader(md metadata.MD) error {
	return s.ServerStream.SendHeader(md)
}
func (s *serverTransportStream) SetTrailer(md metadata.MD) error {
	s.ServerStream.SetTrailer(md)
	return nil
}
