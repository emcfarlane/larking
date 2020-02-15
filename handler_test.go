package graphpb

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"

	"github.com/afking/graphpb/google.golang.org/genproto/googleapis/api/httpbody"
	"github.com/afking/graphpb/grpc/reflection"
	"github.com/afking/graphpb/testpb"
)

func TestMessageServer(t *testing.T) {

	// Create test server.
	ms := &testpb.UnimplementedMessagingServer{}
	fs := &testpb.UnimplementedFilesServer{}

	overrides := make(map[string]func(context.Context, proto.Message, string) (proto.Message, error))
	gs := grpc.NewServer(
		grpc.StreamInterceptor(
			streamServerTestInterceptor,
		),
		grpc.UnaryInterceptor(
			//unaryServerTestInterceptor,
			func(
				ctx context.Context,
				req interface{},
				info *grpc.UnaryServerInfo,
				handler grpc.UnaryHandler,
			) (interface{}, error) {
				md, ok := metadata.FromIncomingContext(ctx)
				if !ok {
					return handler(ctx, req) // default
				}
				ss := md["test"]
				if len(ss) == 0 {
					return handler(ctx, req) // default
				}
				h, ok := overrides[ss[0]]
				if !ok {
					return handler(ctx, req) // default
				}

				// TODO: reflection assert on handler types.
				return h(ctx, req.(proto.Message), info.FullMethod)
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
	go gs.Serve(lis)
	defer gs.Stop()

	// Create client.
	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	if err != nil {
		t.Fatalf("cannot connect to server: %v", err)
	}
	defer conn.Close()

	h, err := NewMux(conn)
	if err != nil {
		t.Fatal(err)
	}

	type in struct {
		method string
		msg    proto.Message
		// TODO: headers?
	}

	type out struct {
		msg proto.Message
		err error
		// TODO: trailers?
	}

	type want struct {
		statusCode int
		body       []byte
		// TODO: headers
	}

	// TODO: compare http.Response output
	tests := []struct {
		name string
		req  *http.Request
		in   in
		out  out
		want want
	}{{
		name: "first",
		req:  httptest.NewRequest(http.MethodGet, "/v1/messages/name/hello", nil),
		in: in{
			method: "/graphpb.testpb.Messaging/GetMessageOne",
			msg:    &testpb.GetMessageRequestOne{Name: "name/hello"},
		},
		out: out{
			msg: &testpb.Message{Text: "hello, world!"},
		},
		want: want{
			statusCode: 200,
			body:       []byte(`{"text":"hello, world!"}`),
		},
	}, {
		name: "sub.subfield",
		req:  httptest.NewRequest(http.MethodGet, "/v1/messages/123456?revision=2&sub.subfield=foo", nil),
		in: in{
			method: "/graphpb.testpb.Messaging/GetMessageTwo",
			msg: &testpb.GetMessageRequestTwo{
				MessageId: "123456",
				Revision:  2,
				Sub: &testpb.GetMessageRequestTwo_SubMessage{
					Subfield: "foo",
				},
			},
		},
		out: out{
			msg: &testpb.Message{Text: "hello, query params!"},
		},
		want: want{
			statusCode: 200,
			body:       []byte(`{"text":"hello, query params!"}`),
		},
	}, {
		name: "additional_bindings",
		req:  httptest.NewRequest(http.MethodGet, "/v1/users/usr_123/messages/msg_123?revision=2", nil),
		in: in{
			method: "/graphpb.testpb.Messaging/GetMessageTwo",
			msg: &testpb.GetMessageRequestTwo{
				MessageId: "msg_123",
				Revision:  2,
				UserId:    "usr_123",
			},
		},
		out: out{
			msg: &testpb.Message{Text: "hello, additional bindings!"},
		},
		want: want{
			statusCode: 200,
			body:       []byte(`{"text":"hello, additional bindings!"}`),
		},
	}, {
		name: "patch",
		req: httptest.NewRequest(http.MethodPatch, "/v1/messages/msg_123", strings.NewReader(
			`{ "text": "Hi!" }`,
		)),
		in: in{
			method: "/graphpb.testpb.Messaging/UpdateMessage",
			msg: &testpb.UpdateMessageRequestOne{
				MessageId: "msg_123",
				Message: &testpb.Message{
					Text: "Hi!",
				},
			},
		},
		out: out{
			msg: &testpb.Message{Text: "hello, patch!"},
		},
		want: want{
			statusCode: 200,
			body:       []byte(`{"text":"hello, patch!"}`),
		},
	}, {
		name: "404",
		req:  httptest.NewRequest(http.MethodGet, "/error404", nil),
		want: want{
			statusCode: 404,
			body:       []byte(`{"code":5, "message":"not found"}`),
		},
	}, {
		name: "cat.jpg",
		req: func() *http.Request {
			r := httptest.NewRequest(
				http.MethodPost, "/files/cat.jpg",
				strings.NewReader("cat"),
			)
			r.Header.Set("Content-Type", "image/jpeg")
			return r
		}(),
		in: in{
			method: "/graphpb.testpb.Files/UploadDownload",
			msg: &testpb.UploadFileRequest{
				Filename: "cat.jpg",
				File: &httpbody.HttpBody{
					ContentType: "image/jpeg",
					Data:        []byte("cat"),
				},
			},
		},
		out: out{
			msg: &httpbody.HttpBody{
				ContentType: "image/jpeg",
				Data:        []byte("cat"),
			},
		},
		want: want{
			statusCode: 200,
			body:       []byte("cat"),
		},
	}}

	opts := cmp.Options{protocmp.Transform()}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overrides[t.Name()] = func(
				ctx context.Context, msg proto.Message, method string,
			) (proto.Message, error) {
				if method != tt.in.method {
					return nil, fmt.Errorf("grpc expected %s, got %s", tt.in.method, method)
				}

				diff := cmp.Diff(msg, tt.in.msg, opts...)
				if diff != "" {
					return nil, fmt.Errorf(diff)
				}
				return tt.out.msg, tt.out.err
			}
			defer delete(overrides, t.Name())

			// ctx hack
			ctx := tt.req.Context()
			ctx = metadata.AppendToOutgoingContext(ctx, "test", t.Name())
			req := tt.req.WithContext(ctx)

			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			resp := w.Result()

			b, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}

			if sc := tt.want.statusCode; sc != resp.StatusCode {
				t.Errorf("expected %d got %d", tt.want.statusCode, resp.StatusCode)
			}

			if !bytes.Equal(b, tt.want.body) {
				t.Errorf("body %s != %s", b, tt.want.body)
			}
		})
	}
}

func unaryServerTestInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (_ interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = status.Errorf(codes.Internal, "%s", r)
		}
	}()
	return handler(ctx, req)
}

// StreamServerInterceptor returns a new streaming server interceptor for panic recovery.
func streamServerTestInterceptor(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = status.Errorf(codes.Internal, "%s", r)
		}
	}()
	return handler(srv, stream)
}
