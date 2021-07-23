// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package larking

import (
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/genproto/googleapis/api/httpbody"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"

	"github.com/emcfarlane/larking/testpb"
	"golang.org/x/sync/errgroup"
)

type override struct {
	unary  func(context.Context, proto.Message, string) (proto.Message, error)
	stream func(grpc.ServerStream, string) error
}

type overrides map[string]override

func (os overrides) fromContext(ctx context.Context) (override, bool) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return override{}, false
	}

	ss := md["test"]
	if len(ss) == 0 {
		return override{}, false
	}
	h, ok := os[ss[0]]
	return h, ok
}

func (os overrides) StreamInterceptor() grpc.ServerOption {
	return grpc.StreamInterceptor(
		func(
			srv interface{},
			stream grpc.ServerStream,
			info *grpc.StreamServerInfo,
			handler grpc.StreamHandler,
		) (err error) {
			ctx := stream.Context()

			h, ok := os.fromContext(ctx)
			if !ok {
				return handler(srv, stream) // default
			}
			return h.stream(stream, info.FullMethod)
		},
	)
}

func (os overrides) UnaryInterceptor() grpc.ServerOption {
	return grpc.UnaryInterceptor(
		func(
			ctx context.Context,
			req interface{},
			info *grpc.UnaryServerInfo,
			handler grpc.UnaryHandler,
		) (interface{}, error) {
			h, ok := os.fromContext(ctx)
			if !ok {
				return handler(ctx, req) // default
			}

			// TODO: reflection assert on handler types.
			return h.unary(ctx, req.(proto.Message), info.FullMethod)
		},
	)
}

func TestGRPCProxy(t *testing.T) {
	// Create test server.
	ms := &testpb.UnimplementedMessagingServer{}
	fs := &testpb.UnimplementedFilesServer{}

	overrides := make(overrides)
	gs := grpc.NewServer(
		overrides.StreamInterceptor(),
		overrides.UnaryInterceptor(),
	)
	testpb.RegisterMessagingServer(gs, ms)
	testpb.RegisterFilesServer(gs, fs)
	reflection.Register(gs)

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()

	var g errgroup.Group
	defer func() {
		if err := g.Wait(); err != nil {
			t.Fatal(err)
		}
	}()

	g.Go(func() error {
		return gs.Serve(lis)
	})
	defer gs.Stop()

	// Create the client.
	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	if err != nil {
		t.Fatalf("cannot connect to server: %v", err)
	}
	defer conn.Close()

	h, err := NewMux()
	if err != nil {
		t.Fatal(err)
	}
	if err := h.RegisterConn(context.Background(), conn); err != nil {
		t.Fatal(err)
	}

	lisProxy, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer lisProxy.Close()

	ts := grpc.NewServer(
		grpc.UnknownServiceHandler(h.StreamHandler()),
	)

	g.Go(func() error {
		return ts.Serve(lisProxy)
	})
	defer ts.Stop()

	cc, err := grpc.Dial(
		lisProxy.Addr().String(),
		//grpc.WithTransportCredentials(
		//	credentials.NewTLS(transport.TLSClientConfig),
		//),
		grpc.WithInsecure(),
	)
	if err != nil {
		t.Fatal(err)
	}

	type in struct {
		msg proto.Message
	}

	type out struct {
		msg proto.Message
		err error
	}

	cmpOpts := cmp.Options{protocmp.Transform()}

	var unaryStreamDesc = &grpc.StreamDesc{
		ClientStreams: false,
		ServerStreams: false,
	}

	tests := []struct {
		name   string
		desc   *grpc.StreamDesc
		method string
		ins    []in
		outs   []out
		//wants []wants
	}{{
		name:   "unary_message",
		desc:   unaryStreamDesc,
		method: "/larking.testpb.Messaging/GetMessageOne",
		ins: []in{{
			msg: &testpb.GetMessageRequestOne{
				Name: "proxy",
			},
		}},
		outs: []out{{
			msg: &testpb.Message{Text: "success"},
		}},
	}, {
		name: "stream_file",
		desc: &grpc.StreamDesc{
			ClientStreams: true,
			ServerStreams: true,
		},
		method: "/larking.testpb.Files/LargeUploadDownload",
		ins: []in{{
			msg: &testpb.UploadFileRequest{
				Filename: "cat.jpg",
				File: &httpbody.HttpBody{
					ContentType: "jpg",
					Data:        []byte("cat"),
				},
			},
		}, {
			msg: &testpb.UploadFileRequest{
				File: &httpbody.HttpBody{
					Data: []byte("dog"),
				},
			},
		}},
		outs: []out{{
			msg: &httpbody.HttpBody{
				Data: []byte("cat"),
			},
		}, {
			msg: &httpbody.HttpBody{
				Data: []byte("dog"),
			},
		}},
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var o override
			if tt.desc.ClientStreams || tt.desc.ServerStreams {
				o.stream = func(stream grpc.ServerStream, method string) error {
					if method != tt.method {
						return fmt.Errorf("grpc expected %s, got %s", tt.method, method)
					}

					var err error
					for i := 0; ; i++ {
						// Consistent type
						in := proto.Clone(tt.ins[0].msg)
						if err = stream.RecvMsg(in); err != nil {
							break
						}

						diff := cmp.Diff(in, tt.ins[i].msg, cmpOpts...)
						if diff != "" {
							return fmt.Errorf(diff)
						}

						out := tt.outs[i].msg
						if err := stream.SendMsg(out); err != nil {
							break
						}
					}
					if isStreamError(err) {
						return err
					}
					return nil
				}
			} else {
				o.unary = func(
					ctx context.Context,
					msg proto.Message,
					method string,
				) (proto.Message, error) {
					if method != tt.method {
						return nil, fmt.Errorf("grpc expected %s, got %s", tt.method, method)
					}

					diff := cmp.Diff(msg, tt.ins[0].msg, cmpOpts...)
					if diff != "" {
						return nil, fmt.Errorf(diff)
					}
					return tt.outs[0].msg, tt.outs[0].err
				}

			}

			overrides[t.Name()] = o
			defer delete(overrides, t.Name())

			ctx := context.Background()
			ctx = metadata.AppendToOutgoingContext(ctx, "test", t.Name())

			s, err := cc.NewStream(ctx, tt.desc, tt.method)
			if err != nil {
				t.Fatal(err)
			}

			for i := 0; i < len(tt.ins); i++ {
				if err := s.SendMsg(tt.ins[i].msg); err != nil {
					t.Fatal(err)
				}

				out := proto.Clone(tt.outs[i].msg)
				if err := s.RecvMsg(out); err != nil {
					t.Fatal(err)
				}

				diff := cmp.Diff(out, tt.outs[i].msg, cmpOpts...)
				if diff != "" {
					t.Fatal(diff)
				}
			}
		})
	}
}
