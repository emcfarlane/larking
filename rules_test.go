// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package larking

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
	"golang.org/x/sync/errgroup"
	"google.golang.org/genproto/googleapis/api/httpbody"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/emcfarlane/larking/testpb"
)

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

// overrides is a map of an array of in/out msgs.
type overrides struct {
	testing.TB
	header string
	inouts []interface{}
}

// unary context is used to check if this request should be overriden.
func (o *overrides) unary() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		if hdr := md[o.header]; len(hdr) == 0 || info.FullMethod != hdr[0] {
			return handler(ctx, req)
		}
		in, out := o.inouts[0].(in), o.inouts[1].(out)

		msg := req.(proto.Message)
		if in.method != "" && info.FullMethod != in.method {
			err := fmt.Errorf("grpc expected %s, got %s", in.method, info.FullMethod)
			o.Log(err)
			return nil, err
		}

		diff := cmp.Diff(msg, in.msg, protocmp.Transform())
		if diff != "" {
			o.Log(diff)
			return nil, fmt.Errorf("message didn't match")
		}
		return out.msg, out.err
	}
}

func (o *overrides) unaryOption() grpc.ServerOption {
	return grpc.UnaryInterceptor(o.unary())
}

// stream context is used to check if this request should be overriden.
func (o *overrides) stream() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		stream grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) (err error) {
		md, _ := metadata.FromIncomingContext(stream.Context())
		if hdr := md[o.header]; len(hdr) == 0 || info.FullMethod != hdr[0] {
			return handler(srv, stream)
		}

		for i, v := range o.inouts {
			switch v := v.(type) {
			case in:
				//if v.method != "" && info.FullMethod != v.method {
				//	return fmt.Errorf("grpc expected %s, got %s", v.method, info.FullMethod)
				//}

				msg := v.msg.ProtoReflect().New().Interface()
				if err := stream.RecvMsg(msg); err != nil {
					o.Log(err)
					return err
				}
				diff := cmp.Diff(msg, v.msg, protocmp.Transform())
				if diff != "" {
					o.Log(diff)
					return fmt.Errorf("message didn't match")
				}

			case out:
				if i == 0 {
					return fmt.Errorf("unexpected first message type: %T", v)
				}

				if err := v.err; err != nil {
					o.Log(err)
					return err // application
				}
				if err := stream.SendMsg(v.msg); err != nil {
					o.Log(err)
					return err
				}
			default:
				return fmt.Errorf("unknown override type: %T", v)
			}
		}
		return nil
	}
}

func (o *overrides) streamOption() grpc.ServerOption {
	return grpc.StreamInterceptor(o.stream())
}

func (o *overrides) reset(t testing.TB, header string, msgs []interface{}) {
	o.TB = t
	o.header = header
	o.inouts = append(o.inouts[:0], msgs...)
}

func TestMessageServer(t *testing.T) {

	// Create test server.
	ms := &testpb.UnimplementedMessagingServer{}
	fs := &testpb.UnimplementedFilesServer{}
	js := &testpb.UnimplementedWellKnownServer{}

	o := new(overrides)
	gs := grpc.NewServer(o.unaryOption(), o.streamOption())

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
	conn, err := grpc.Dial(
		lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
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
			method: "/larking.testpb.Messaging/GetMessageOne",
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
			method: "/larking.testpb.Messaging/GetMessageTwo",
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
			method: "/larking.testpb.Messaging/GetMessageTwo",
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
			method: "/larking.testpb.Messaging/GetMessageTwo",
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
			method: "/larking.testpb.Messaging/UpdateMessage",
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
			method: "/larking.testpb.Messaging/Action",
			msg:    &testpb.Message{MessageId: "123", Text: "action"},
		},
		out: out{
			msg: &emptypb.Empty{},
		},
		want: want{
			statusCode: 200,
			msg:        &emptypb.Empty{},
		},
	}, {
		name: "actionSegment",
		req: httptest.NewRequest(http.MethodPost, "/v1/name:clear", strings.NewReader(
			`{ "message_id": "123" }`,
		)),
		in: in{
			method: "/larking.testpb.Messaging/ActionSegment",
			msg:    &testpb.Message{MessageId: "123", Text: "name"},
		},
		out: out{
			msg: &emptypb.Empty{},
		},
		want: want{
			statusCode: 200,
			msg:        &emptypb.Empty{},
		},
	}, {
		name: "actionResource",
		req:  httptest.NewRequest(http.MethodGet, "/v1/actions/123:fetch", nil),
		in: in{
			method: "/larking.testpb.Messaging/ActionResource",
			msg:    &testpb.Message{Text: "actions/123"},
		},
		out: out{
			msg: &emptypb.Empty{},
		},
		want: want{
			statusCode: 200,
			msg:        &emptypb.Empty{},
		},
	}, {
		name: "actionSegments",
		req: httptest.NewRequest(http.MethodPost, "/v1/name/id:watch", strings.NewReader(
			`{ "message_id": "123" }`,
		)),
		in: in{
			method: "/larking.testpb.Messaging/ActionSegments",
			msg:    &testpb.Message{MessageId: "123", Text: "name/id"},
		},
		out: out{
			msg: &emptypb.Empty{},
		},
		want: want{
			statusCode: 200,
			msg:        &emptypb.Empty{},
		},
	}, {
		name: "batchGet",
		req: httptest.NewRequest(http.MethodGet, "/v3/events:batchGet", strings.NewReader(
			`{}`,
		)),
		in: in{
			method: "/larking.testpb.Messaging/BatchGet",
			msg:    &emptypb.Empty{},
		},
		out: out{
			msg: &emptypb.Empty{},
		},
		want: want{
			statusCode: 200,
			msg:        &emptypb.Empty{},
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
			method: "/larking.testpb.Files/UploadDownload",
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

		/*}, {
		name: "large_cat.jpg",
		req: func() *http.Request {
			r := httptest.NewRequest(
				http.MethodPost, "/files/large/cat.jpg",
				strings.NewReader("cat"),
			)
			r.Header.Set("Content-Type", "image/jpeg")
			return r
		}(),
		in: in{
			method: "/larking.testpb.Files/UploadDownload",
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
		},*/
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
				"string_value=hello&"+
				"field_mask=\"user.displayName,photo\"",
			nil,
		),
		in: in{
			method: "/larking.testpb.WellKnown/Check",
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
				FieldMask: &fieldmaskpb.FieldMask{
					Paths: []string{
						"user.display_name", // JSON name converted to field name
						"photo",
					},
				},
			},
		},
		out: out{
			msg: &emptypb.Empty{},
		},
		want: want{
			statusCode: 200,
			msg:        &emptypb.Empty{},
		},
	}, {
		name: "variable_one",
		req:  httptest.NewRequest(http.MethodGet, "/version/one", nil),
		in: in{
			method: "/larking.testpb.Messaging/VariableOne",
			msg:    &testpb.Message{Text: "version"},
		},
		out: out{
			msg: &emptypb.Empty{},
		},
		want: want{
			statusCode: 200,
			msg:        &emptypb.Empty{},
		},
	}, {
		name: "variable_two",
		req:  httptest.NewRequest(http.MethodGet, "/version/two", nil),
		in: in{
			method: "/larking.testpb.Messaging/VariableTwo",
			msg:    &testpb.Message{Text: "version"},
		},
		out: out{
			msg: &emptypb.Empty{},
		},
		want: want{
			statusCode: 200,
			msg:        &emptypb.Empty{},
		},
	}, {
		name: "shelf_name_get",
		req:  httptest.NewRequest(http.MethodGet, "/v1/shelves/shelf1", nil),
		in: in{
			method: "/larking.testpb.Messaging/GetShelf",
			msg:    &testpb.GetShelfRequest{Name: "shelves/shelf1"},
		},
		out: out{
			msg: &testpb.Shelf{Name: "shelves/shelf1"},
		},
		want: want{
			statusCode: 200,
			msg:        &testpb.Shelf{Name: "shelves/shelf1"},
		},
	}, {
		name: "book_name_get",
		req:  httptest.NewRequest(http.MethodGet, "/v1/shelves/shelf1/books/book2", nil),
		in: in{
			method: "/larking.testpb.Messaging/GetBook",
			msg:    &testpb.GetBookRequest{Name: "shelves/shelf1/books/book2"},
		},
		out: out{
			msg: &testpb.Book{Name: "shelves/shelf1/books/book2"},
		},
		want: want{
			statusCode: 200,
			msg:        &testpb.Book{Name: "shelves/shelf1/books/book2"},
		},
	}, {
		name: "book_name_create",
		req: httptest.NewRequest(http.MethodPost, "/v1/shelves/shelf1/books", strings.NewReader(
			`{ "name": "book3" }`,
		)),
		in: in{
			method: "/larking.testpb.Messaging/CreateBook",
			msg: &testpb.CreateBookRequest{
				Parent: "shelves/shelf1",
				Book: &testpb.Book{
					Name: "book3",
				},
			},
		},
		out: out{
			msg: &testpb.Book{Name: "book3"},
		},
		want: want{
			statusCode: 200,
			msg:        &testpb.Book{Name: "book3"},
		},
	}, {
		name: "book_name_update",
		req: httptest.NewRequest(http.MethodPatch, `/v1/shelves/shelf1/books/book2?update_mask="name,title"`, strings.NewReader(
			`{ "title": "Lord of the Rings" }`,
		)),
		in: in{
			method: "/larking.testpb.Messaging/UpdateBook",
			msg: &testpb.UpdateBookRequest{
				Book: &testpb.Book{
					Name:  "shelves/shelf1/books/book2",
					Title: "Lord of the Rings",
				},
				UpdateMask: &fieldmaskpb.FieldMask{
					Paths: []string{
						"name",
						"title",
					},
				},
			},
		},
		out: out{
			msg: &testpb.Book{
				Name:  "shelves/shelf1/books/book2",
				Title: "Lord of the Rings",
			},
		},
		want: want{
			statusCode: 200,
			msg: &testpb.Book{
				Name:  "shelves/shelf1/books/book2",
				Title: "Lord of the Rings",
			},
		},
	}}

	opts := cmp.Options{protocmp.Transform()}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o.reset(t, "http-test", []interface{}{tt.in, tt.out})

			req := tt.req
			req.Header["test"] = []string{tt.in.method}

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
