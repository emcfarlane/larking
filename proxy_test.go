package graphpb

import (
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/genproto/googleapis/api/httpbody"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"

	"github.com/afking/graphpb/grpc/reflection"
	"github.com/afking/graphpb/testpb"
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

func TestGRPCProxy(t *testing.T) {
	// Create test server.
	ms := &testpb.UnimplementedMessagingServer{}
	fs := &testpb.UnimplementedFilesServer{}

	overrides := make(overrides)
	gs := grpc.NewServer(
		grpc.StreamInterceptor(
			func(
				srv interface{},
				stream grpc.ServerStream,
				info *grpc.StreamServerInfo,
				handler grpc.StreamHandler,
			) (err error) {
				ctx := stream.Context()

				h, ok := overrides.fromContext(ctx)
				if !ok {
					return handler(srv, stream) // default
				}
				return h.stream(stream, info.FullMethod)
			},
		),
		grpc.UnaryInterceptor(
			func(
				ctx context.Context,
				req interface{},
				info *grpc.UnaryServerInfo,
				handler grpc.UnaryHandler,
			) (interface{}, error) {
				h, ok := overrides.fromContext(ctx)
				if !ok {
					return handler(ctx, req) // default
				}

				// TODO: reflection assert on handler types.
				return h.unary(ctx, req.(proto.Message), info.FullMethod)
			},
		),
	)
	testpb.RegisterMessagingServer(gs, ms)
	testpb.RegisterFilesServer(gs, fs)
	reflection.Register(gs)

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()

	go gs.Serve(lis)
	defer gs.Stop()

	// Create the client.
	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	if err != nil {
		t.Fatalf("cannot connect to server: %v", err)
	}
	defer conn.Close()

	h, err := NewMux(conn)
	if err != nil {
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

	go ts.Serve(lisProxy)
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
		method: "/graphpb.testpb.Messaging/GetMessageOne",
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
		method: "/graphpb.testpb.Files/LargeUploadDownload",
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
			var i int

			var o override
			if tt.desc.ClientStreams || tt.desc.ServerStreams {
				o.stream = func(stream grpc.ServerStream, method string) error {
					if method != tt.method {
						return fmt.Errorf("grpc expected %s, got %s", tt.method, method)
					}

					var err error
					for {
						in := proto.Clone(tt.ins[i].msg)
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
						//fmt.Println("sent!", i)
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

					diff := cmp.Diff(msg, tt.ins[i].msg, cmpOpts...)
					if diff != "" {
						return nil, fmt.Errorf(diff)
					}
					return tt.outs[i].msg, tt.outs[i].err
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

			// len(ins) == len(outs)
			for ; i < len(tt.ins); i++ {
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
