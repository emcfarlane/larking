package graphpb

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"

	"github.com/afking/graphpb/grpc/reflection"
	"github.com/afking/graphpb/mock_testpb"
	"github.com/afking/graphpb/testpb"
)

func TestMessageServer(t *testing.T) {

	// Create test server.
	var ts = new(struct{ testpb.MessagingServer }) // override each test

	gs := grpc.NewServer(
		grpc.StreamInterceptor(
			streamServerTestInterceptor,
		),
		grpc.UnaryInterceptor(
			unaryServerTestInterceptor,
		),
	)
	testpb.RegisterMessagingServer(gs, ts)
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

	opts := cmp.Options{protocmp.Transform()}

	// TODO: compare http.Response output
	tests := []struct {
		name   string
		req    *http.Request
		expect func(t *testing.T, ms *mock_testpb.MockMessagingServer)
		//in, out    proto.Message
		//method     func(arg0, arg1 interface{}) *gomock.Call
		//err        error
		statusCode int
	}{{
		name: "first",
		req:  httptest.NewRequest(http.MethodGet, "/v1/messages/name/hello", nil),
		expect: func(t *testing.T, ms *mock_testpb.MockMessagingServer) {
			ms.EXPECT().GetMessageOne(
				gomock.Any(),
				protoMatches{&testpb.GetMessageRequestOne{
					Name: "name/hello",
				}, opts},
			).Return(&testpb.Message{Text: "hello, world!"}, nil)
		},
		statusCode: 200,
	}, {
		name: "sub.subfield",
		req:  httptest.NewRequest(http.MethodGet, "/v1/messages/123456?revision=2&sub.subfield=foo", nil),
		expect: func(t *testing.T, ms *mock_testpb.MockMessagingServer) {
			ms.EXPECT().GetMessageTwo(
				gomock.Any(),
				protoMatches{&testpb.GetMessageRequestTwo{
					MessageId: "123456",
					Revision:  2,
					Sub: &testpb.GetMessageRequestTwo_SubMessage{
						Subfield: "foo",
					},
				}, opts},
			).Return(&testpb.Message{Text: "hello, query params!"}, nil)
		},
		statusCode: 200,
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(&mockReporter{T: t})
			defer ctrl.Finish()

			ms := mock_testpb.NewMockMessagingServer(ctrl)
			ts.MessagingServer = ms
			tt.expect(t, ms)

			w := httptest.NewRecorder()
			h.ServeHTTP(w, tt.req)
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
	T    *testing.T
	done bool
}

func (x mockReporter) Helper()                                   { x.T.Helper() }
func (x mockReporter) Errorf(format string, args ...interface{}) { x.T.Errorf(format, args...) }
func (x *mockReporter) Fatalf(format string, args ...interface{}) {
	if !x.done {
		x.done = true
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

func (p protoMatches) Matches(x interface{}) bool {
	return cmp.Diff(p.msg, x, p.opts...) == ""
}
func (p protoMatches) String() string { return fmt.Sprint(p.msg) }
