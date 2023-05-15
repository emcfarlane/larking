package larking

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding"
	grpc_testing "google.golang.org/grpc/interop/grpc_testing"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestGRPC(t *testing.T) {
	// Create test server.
	ts := grpc_testing.UnimplementedTestServiceServer{}

	o := new(overrides)
	m, err := NewMux(
		UnaryServerInterceptorOption(o.unary()),
		StreamServerInterceptorOption(o.stream()),
	)
	if err != nil {
		t.Fatalf("failed to create mux: %v", err)
	}
	grpc_testing.RegisterTestServiceServer(m, ts)

	index := http.HandlerFunc(m.serveGRPC)

	h2s := &http2.Server{}
	hs := &http.Server{
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1 MB
		Handler:        h2c.NewHandler(index, h2s),
	}
	if err := http2.ConfigureServer(hs, h2s); err != nil {
		t.Fatalf("failed to configure server: %v", err)
	}

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()

	// Start server.
	go hs.Serve(lis)
	defer hs.Close()

	encoding.RegisterCompressor(&CompressorGzip{})

	conns := []struct {
		name string
		opts []grpc.DialOption
	}{{
		name: "insecure",
		opts: []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
		},
	}, {
		name: "compressed",
		opts: []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
			grpc.WithDefaultCallOptions(grpc.UseCompressor("gzip")),
		},
	}}

	type want struct {
		statusCode int
	}

	// https://github.com/grpc/grpc/blob/master/src/proto/grpc/testing/test.proto
	tests := []struct {
		name   string
		method string
		desc   grpc.StreamDesc
		want   want
		inouts []any
	}{{
		name:   "unary",
		method: "/grpc.testing.TestService/UnaryCall",
		desc:   grpc.StreamDesc{},
		inouts: []any{
			in{
				msg: &grpc_testing.SimpleRequest{
					Payload: &grpc_testing.Payload{Body: []byte{0}},
				},
			},
			out{
				msg: &grpc_testing.SimpleResponse{
					Payload: &grpc_testing.Payload{Body: []byte{0}},
				},
			},
		},
	}, {
		name:   "client_streaming",
		method: "/grpc.testing.TestService/StreamingInputCall",
		desc: grpc.StreamDesc{
			ClientStreams: true,
		},
		inouts: []any{
			in{
				msg: &grpc_testing.StreamingInputCallRequest{
					Payload: &grpc_testing.Payload{Body: []byte{0}},
				},
			},
			in{
				msg: &grpc_testing.StreamingInputCallRequest{
					Payload: &grpc_testing.Payload{Body: []byte{0}},
				},
			},
			out{
				msg: &grpc_testing.StreamingInputCallResponse{
					AggregatedPayloadSize: 2,
				},
			},
		},
	}, {
		name:   "server_streaming",
		method: "/grpc.testing.TestService/StreamingOutputCall",
		desc: grpc.StreamDesc{
			ServerStreams: true,
		},
		inouts: []any{
			in{
				msg: &grpc_testing.StreamingOutputCallRequest{
					Payload: &grpc_testing.Payload{Body: []byte{0}},
				},
			},
			out{
				msg: &grpc_testing.StreamingOutputCallResponse{
					Payload: &grpc_testing.Payload{Body: []byte{0}},
				},
			},
			out{
				msg: &grpc_testing.StreamingOutputCallResponse{
					Payload: &grpc_testing.Payload{Body: []byte{0}},
				},
			},
		},
	}, {
		name:   "full_streaming",
		method: "/grpc.testing.TestService/FullDuplexCall",
		desc: grpc.StreamDesc{
			ClientStreams: true,
			ServerStreams: true,
		},
		inouts: []any{
			in{
				msg: &grpc_testing.StreamingOutputCallRequest{
					Payload: &grpc_testing.Payload{Body: []byte{0}},
				},
			},
			out{
				msg: &grpc_testing.StreamingOutputCallResponse{
					Payload: &grpc_testing.Payload{Body: []byte{0}},
				},
			},
			in{
				msg: &grpc_testing.StreamingOutputCallRequest{
					Payload: &grpc_testing.Payload{Body: []byte{0}},
				},
			},
			out{
				msg: &grpc_testing.StreamingOutputCallResponse{
					Payload: &grpc_testing.Payload{Body: []byte{0}},
				},
			},
		},
	}, {
		name:   "half_streaming",
		method: "/grpc.testing.TestService/HalfDuplexCall",
		desc: grpc.StreamDesc{
			ClientStreams: true,
			ServerStreams: true,
		},
		inouts: []any{
			in{
				msg: &grpc_testing.StreamingOutputCallRequest{
					Payload: &grpc_testing.Payload{Body: []byte{0}},
				},
			},
			in{
				msg: &grpc_testing.StreamingOutputCallRequest{
					Payload: &grpc_testing.Payload{Body: []byte{0}},
				},
			},
			out{
				msg: &grpc_testing.StreamingOutputCallResponse{
					Payload: &grpc_testing.Payload{Body: []byte{0}},
				},
			},
			out{
				msg: &grpc_testing.StreamingOutputCallResponse{
					Payload: &grpc_testing.Payload{Body: []byte{0}},
				},
			},
		},
	}, {
		name:   "large_client_streaming",
		method: "/grpc.testing.TestService/StreamingInputCall",
		desc: grpc.StreamDesc{
			ClientStreams: true,
		},
		inouts: []any{
			in{
				msg: &grpc_testing.StreamingInputCallRequest{
					Payload: &grpc_testing.Payload{Body: make([]byte, 1024*1024)}, //1024*1024)},
				},
			},
			in{
				msg: &grpc_testing.StreamingInputCallRequest{
					Payload: &grpc_testing.Payload{Body: make([]byte, 1024*1024)}, //1024*1024)},
				},
			},
			out{
				msg: &grpc_testing.StreamingInputCallResponse{
					AggregatedPayloadSize: 2,
				},
			},
		},
	}}

	opts := cmp.Options{protocmp.Transform()}
	for _, tc := range conns {
		t.Run(tc.name, func(t *testing.T) {
			conn, err := grpc.Dial(lis.Addr().String(), tc.opts...)
			if err != nil {
				t.Fatalf("failed to dial: %v", err)
			}
			defer conn.Close()

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					o.reset(t, "test", tt.inouts)

					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()

					ctx = metadata.AppendToOutgoingContext(ctx, "test", tt.method)

					stream, err := conn.NewStream(ctx, &tt.desc, tt.method)
					if err != nil {
						t.Fatalf("failed to create stream: %v", err)
					}

					for i, inout := range tt.inouts {

						switch v := inout.(type) {
						case in:
							t.Logf("stream.SendMsg: %d", i)
							if err := stream.SendMsg(v.msg); err != nil {
								t.Fatalf("failed to send msg: %v", err)
							}
						case out:
							t.Logf("stream.RecvMsg: %d", i)
							want := v.msg
							got := v.msg.ProtoReflect().New().Interface()
							if err := stream.RecvMsg(got); err != nil {
								t.Fatalf("failed to recv msg: %v", err)
							}
							diff := cmp.Diff(got, want, opts...)
							if diff != "" {
								t.Error(diff)
							}
						}
					}
					t.Logf("stream: %+v", stream)
				})
			}
		})
	}
}
