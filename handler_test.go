package graphpb

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"

	"github.com/afking/graphpb/grpc/reflection"
	"github.com/afking/graphpb/mock_testpb"
	"github.com/afking/graphpb/testpb"
)

func TestMessageServer(t *testing.T) {

	// Create test server.
	//var ts = new(struct{ testpb.MessagingServer }) // override each test
	ctrl := gomock.NewController(&mockReporter{T: t, id: getGID()})
	defer ctrl.Finish()

	ms := mock_testpb.NewMockMessagingServer(ctrl)

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
			) (_ interface{}, err error) {
				defer func() {
					if r := recover(); r != nil {
						err = status.Errorf(codes.Internal, "%s", r)
					}
				}()

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

	// TODO: compare http.Response output
	tests := []struct {
		name       string
		req        *http.Request
		in         in
		out        out
		statusCode int
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
		statusCode: 200,
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
		statusCode: 200,
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
		statusCode: 200,
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
			msg: &testpb.Message{Text: "hello, additional bindings!"},
		},
		statusCode: 200,
	}, {
		name:       "404",
		req:        httptest.NewRequest(http.MethodGet, "/error404", nil),
		statusCode: 404,
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

			if tt.statusCode != resp.StatusCode {
				t.Errorf("expected %d got %d", tt.statusCode, resp.StatusCode)
				t.Fatal(string(b))
			}
		})
	}
}

type mockReporter struct {
	T  *testing.T
	id uint64
}

func (x mockReporter) Helper()                                   { x.T.Helper() }
func (x mockReporter) Errorf(format string, args ...interface{}) { x.T.Errorf(format, args...) }
func (x *mockReporter) Fatalf(format string, args ...interface{}) {
	if getGID() != x.id {
		panic(fmt.Sprintf(format, args...))
	}
	x.T.Fatalf(format, args...)
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

type protoMatches struct {
	msg  proto.Message
	opts []cmp.Option
}

func (p protoMatches) Matches(x interface{}) bool { return cmp.Diff(p.msg, x, p.opts...) == "" }
func (p protoMatches) String() string             { return fmt.Sprint(p.msg) }

// :(
func getGID() uint64 {
	b := make([]byte, 64)
	b = b[:runtime.Stack(b, false)]
	b = bytes.TrimPrefix(b, []byte("goroutine "))
	b = b[:bytes.IndexByte(b, ' ')]
	n, _ := strconv.ParseUint(string(b), 10, 64)
	return n
}
