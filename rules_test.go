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

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/google/go-cmp/cmp"
	"golang.org/x/sync/errgroup"
	"google.golang.org/genproto/googleapis/api/httpbody"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/emcfarlane/graphpb/grpc/reflection"
	"github.com/emcfarlane/graphpb/testpb"
)

func TestMessageServer(t *testing.T) {

	// Create test server.
	ms := &testpb.UnimplementedMessagingServer{}
	fs := &testpb.UnimplementedFilesServer{}
	js := &testpb.UnimplementedWellKnownServer{}

	overrides := make(map[string]func(context.Context, proto.Message, string) (proto.Message, error))
	gs := grpc.NewServer(
		grpc.StreamInterceptor(
			func(
				srv interface{},
				stream grpc.ServerStream,
				info *grpc.StreamServerInfo,
				handler grpc.StreamHandler,
			) (err error) {
				return handler(srv, stream)
			},
		),
		grpc.UnaryInterceptor(
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
	testpb.RegisterWellKnownServer(gs, js)
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
		body       []byte        // either
		msg        proto.Message // or
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
			msg:        &testpb.Message{Text: "hello, world!"},
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
			msg:        &testpb.Message{Text: "hello, query params!"},
		},
	}, {
		name: "additional_bindings1",
		req:  httptest.NewRequest(http.MethodGet, "/v1/users/usr_123/messages?message_id=msg_123&revision=2", nil),
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
			msg:        &testpb.Message{Text: "hello, additional bindings!"},
		},
	}, {
		name: "additional_bindings2",
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
			msg:        &testpb.Message{Text: "hello, additional bindings!"},
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
			msg:        &testpb.Message{Text: "hello, patch!"},
		},
	}, {
		name: "action",
		req: httptest.NewRequest(http.MethodPost, "/v1/action:cancel", strings.NewReader(
			`{ "message_id": "123" }`,
		)),
		in: in{
			method: "/graphpb.testpb.Messaging/Action",
			msg:    &testpb.Message{MessageId: "123", Text: "action"},
		},
		out: out{
			msg: &empty.Empty{},
		},
		want: want{
			statusCode: 200,
			msg:        &empty.Empty{},
		},
	}, {
		name: "404",
		req:  httptest.NewRequest(http.MethodGet, "/error404", nil),
		want: want{
			statusCode: 404,
			msg:        &status.Status{Code: 5, Message: "not found"},
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
	}, {
		name: "wellknown_scalars",
		req: httptest.NewRequest(
			http.MethodGet,
			"/v1/wellknown?"+
				"timestamp=\"2017-01-15T01:30:15.01Z\"&"+
				"duration=\"3.000001s\"&"+
				"bool_value=true&"+
				"int32_value=1&"+
				"int64_value=2&"+
				"uint32_value=3&"+
				"uint64_value=4&"+
				"float_value=5.5&"+
				"double_value=6.6&"+
				"bytes_value=aGVsbG8&"+ // base64URL
				"string_value=hello",
			//"fieldmask=\"user.displayName,photo\"&"+
			nil,
		),
		in: in{
			method: "/graphpb.testpb.WellKnown/Check",
			msg: &testpb.Scalars{
				Timestamp: &timestamppb.Timestamp{
					Seconds: 1484443815,
					Nanos:   10000000,
				},
				Duration: &durationpb.Duration{
					Seconds: 3,
					Nanos:   1000,
				},
				BoolValue:   &wrapperspb.BoolValue{Value: true},
				Int32Value:  &wrapperspb.Int32Value{Value: 1},
				Int64Value:  &wrapperspb.Int64Value{Value: 2},
				Uint32Value: &wrapperspb.UInt32Value{Value: 3},
				Uint64Value: &wrapperspb.UInt64Value{Value: 4},
				FloatValue:  &wrapperspb.FloatValue{Value: 5.5},
				DoubleValue: &wrapperspb.DoubleValue{Value: 6.6},
				BytesValue:  &wrapperspb.BytesValue{Value: []byte("hello")},
				StringValue: &wrapperspb.StringValue{Value: "hello"},
			},
		},
		out: out{
			msg: &emptypb.Empty{},
		},
		want: want{
			statusCode: 200,
			msg:        &emptypb.Empty{},
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
				var msg status.Status
				if err := protojson.Unmarshal(b, &msg); err != nil {
					t.Error(err, string(b))
					return
				}
				t.Error("status.code", msg.Code)
				t.Error("status.message", msg.Message)
				return
			}

			if tt.want.body != nil {
				if !bytes.Equal(b, tt.want.body) {
					t.Errorf("body %s != %s", tt.want.body, b)
				}
			}

			if tt.want.msg != nil {
				msg := proto.Clone(tt.want.msg)
				if err := protojson.Unmarshal(b, msg); err != nil {
					t.Error(err, string(b))
					return
				}

				diff := cmp.Diff(msg, tt.want.msg, opts...)
				if diff != "" {
					t.Error(diff)
				}
			}
		})
	}
}
