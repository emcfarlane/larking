// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package larking

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/sync/errgroup"
	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/genproto/googleapis/api/httpbody"
	"google.golang.org/genproto/googleapis/api/serviceconfig"
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

	"larking.io/api/testpb"
)

type in struct {
	msg    proto.Message
	method string
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
	cs := &testpb.UnimplementedComplexServer{}

	o := new(overrides)
	gs := grpc.NewServer(o.unaryOption(), o.streamOption())

	testpb.RegisterMessagingServer(gs, ms)
	testpb.RegisterFilesServer(gs, fs)
	testpb.RegisterWellKnownServer(gs, js)
	testpb.RegisterComplexServer(gs, cs)
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

	maxSize := 128
	h, err := NewMux(
		MaxReceiveMessageSizeOption(maxSize),
		MaxSendMessageSizeOption(maxSize+2),
		ServiceConfigOption(&serviceconfig.Service{
			Http: &annotations.Http{Rules: []*annotations.HttpRule{{
				Selector: "larking.testpb.Messaging.GetMessageOne",
				Pattern: &annotations.HttpRule_Patch{
					Patch: "/v1/messages/{name=*}:serviceConfig",
				},
			}}},
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.RegisterConn(context.Background(), conn); err != nil {
		t.Fatal(err)
	}

	type want struct {
		msg        proto.Message
		body       []byte
		statusCode int
	}

	// TODO: compare http.Response output
	tests := []struct {
		name   string
		inouts []any
		want   want
		req    *http.Request
	}{{
		name: "first",
		req:  httptest.NewRequest(http.MethodGet, "/v1/messages/name/hello", nil),
		inouts: []any{
			in{
				method: "/larking.testpb.Messaging/GetMessageOne",
				msg:    &testpb.GetMessageRequestOne{Name: "name/hello"},
			},
			out{
				msg: &testpb.Message{Text: "hello, world!"},
			},
		},
		want: want{
			statusCode: 200,
			msg:        &testpb.Message{Text: "hello, world!"},
		},
	}, {
		name: "serviceConfig",
		req:  httptest.NewRequest(http.MethodPatch, "/v1/messages/hello:serviceConfig", nil),
		inouts: []any{
			in{
				method: "/larking.testpb.Messaging/GetMessageOne",
				msg:    &testpb.GetMessageRequestOne{Name: "hello"},
			},
			out{
				msg: &testpb.Message{Text: "hello, world!"},
			},
		},
		want: want{
			statusCode: 200,
			msg:        &testpb.Message{Text: "hello, world!"},
		},
	}, {
		name: "sub.subfield",
		req:  httptest.NewRequest(http.MethodGet, "/v1/messages/123456?revision=2&sub.subfield=foo", nil),
		inouts: []any{
			in{
				method: "/larking.testpb.Messaging/GetMessageTwo",
				msg: &testpb.GetMessageRequestTwo{
					MessageId: "123456",
					Revision:  2,
					Sub: &testpb.GetMessageRequestTwo_SubMessage{
						Subfield: "foo",
					},
				},
			},
			out{
				msg: &testpb.Message{Text: "hello, query params!"},
			},
		},
		want: want{
			statusCode: 200,
			msg:        &testpb.Message{Text: "hello, query params!"},
		},
	}, {
		name: "additional_bindings1",
		req:  httptest.NewRequest(http.MethodGet, "/v1/users/usr_123/messages?message_id=msg_123&revision=2", nil),
		inouts: []any{
			in{
				method: "/larking.testpb.Messaging/GetMessageTwo",
				msg: &testpb.GetMessageRequestTwo{
					MessageId: "msg_123",
					Revision:  2,
					UserId:    "usr_123",
				},
			},
			out{
				msg: &testpb.Message{Text: "hello, additional bindings!"},
			},
		},
		want: want{
			statusCode: 200,
			msg:        &testpb.Message{Text: "hello, additional bindings!"},
		},
	}, {
		name: "additional_bindings2",
		req:  httptest.NewRequest(http.MethodGet, "/v1/users/usr_123/messages/msg_123?revision=2", nil),
		inouts: []any{in{
			method: "/larking.testpb.Messaging/GetMessageTwo",
			msg: &testpb.GetMessageRequestTwo{
				MessageId: "msg_123",
				Revision:  2,
				UserId:    "usr_123",
			},
		},
			out{
				msg: &testpb.Message{Text: "hello, additional bindings!"},
			},
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
		inouts: []any{
			in{
				method: "/larking.testpb.Messaging/UpdateMessage",
				msg: &testpb.UpdateMessageRequestOne{
					MessageId: "msg_123",
					Message: &testpb.Message{
						Text: "Hi!",
					},
				},
			},
			out{
				msg: &testpb.Message{Text: "hello, patch!"},
			},
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
		inouts: []any{

			in{
				method: "/larking.testpb.Messaging/Action",
				msg:    &testpb.Message{MessageId: "123", Text: "action"},
			},
			out{
				msg: &emptypb.Empty{},
			},
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
		inouts: []any{
			in{
				method: "/larking.testpb.Messaging/ActionSegment",
				msg:    &testpb.Message{MessageId: "123", Text: "name"},
			},
			out{
				msg: &emptypb.Empty{},
			},
		},
		want: want{
			statusCode: 200,
			msg:        &emptypb.Empty{},
		},
	}, {
		name: "actionResource",
		req:  httptest.NewRequest(http.MethodGet, "/v1/actions/123:fetch", nil),
		inouts: []any{
			in{
				method: "/larking.testpb.Messaging/ActionResource",
				msg:    &testpb.Message{Text: "actions/123"},
			},
			out{
				msg: &emptypb.Empty{},
			},
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
		inouts: []any{
			in{
				method: "/larking.testpb.Messaging/ActionSegments",
				msg:    &testpb.Message{MessageId: "123", Text: "name/id"},
			},
			out{
				msg: &emptypb.Empty{},
			},
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
		inouts: []any{
			in{
				method: "/larking.testpb.Messaging/BatchGet",
				msg:    &emptypb.Empty{},
			},
			out{
				msg: &emptypb.Empty{},
			},
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
		inouts: []any{
			in{
				method: "/larking.testpb.Files/UploadDownload",
				msg: &testpb.UploadFileRequest{
					Filename: "cat.jpg",
					File: &httpbody.HttpBody{
						ContentType: "image/jpeg",
						Data:        []byte("cat"),
					},
				},
			},
			out{
				msg: &httpbody.HttpBody{
					ContentType: "image/jpeg",
					Data:        []byte("cat"),
				},
			},
		},
		want: want{
			statusCode: 200,
			body:       []byte("cat"),
		},
	}, {
		name: "large_cat.jpg",
		req: func() *http.Request {
			r := httptest.NewRequest(
				http.MethodPost, "/files/large/cat.jpg",
				strings.NewReader("cat"),
			)
			r.Header.Set("Content-Type", "image/jpeg")
			return r
		}(),
		inouts: []any{
			in{
				method: "/larking.testpb.Files/LargeUploadDownload",
				msg: &testpb.UploadFileRequest{
					Filename: "cat.jpg",
					File: &httpbody.HttpBody{
						ContentType: "image/jpeg",
						Data:        []byte("cat"),
					},
				},
			},
			out{
				msg: &httpbody.HttpBody{
					ContentType: "image/jpeg",
					Data:        []byte("cat"),
				},
			},
		},
		want: want{
			statusCode: 200,
			body:       []byte("cat"),
		},
	}, {
		name: "huge_cat.jpg",
		req: func() *http.Request {
			r := httptest.NewRequest(
				http.MethodPost, "/files/large/huge_cat.jpg",
				strings.NewReader(
					"c"+strings.Repeat("a", maxSize)+"t",
				),
			)
			r.Header.Set("Content-Type", "image/jpeg")
			return r
		}(),
		inouts: []any{
			in{
				method: "/larking.testpb.Files/LargeUploadDownload",
				msg: &testpb.UploadFileRequest{
					Filename: "huge_cat.jpg",
					File: &httpbody.HttpBody{
						ContentType: "image/jpeg",
						Data: []byte(
							"c" + strings.Repeat("a", maxSize-1),
						),
					},
				},
			},
			in{
				msg: &testpb.UploadFileRequest{
					File: &httpbody.HttpBody{
						ContentType: "image/jpeg",
						Data:        []byte("at"),
					},
				},
			},
			out{
				msg: &httpbody.HttpBody{
					ContentType: "image/jpeg",
					Data: []byte(
						"c" + strings.Repeat("a", maxSize) + "t",
					),
				},
			},
		},
		want: want{
			statusCode: 200,
			body:       []byte("c" + strings.Repeat("a", maxSize) + "t"),
		},
	}, {
		name: "wellknown_scalars",
		req: httptest.NewRequest(
			http.MethodGet,
			"/v1/wellknown?"+
				url.Values{
					"timestamp":    []string{"2017-01-15T01:30:15.01Z"},
					"duration":     []string{"3.000001s"},
					"bool_value":   []string{"true"},
					"int32_value":  []string{"1"},
					"int64_value":  []string{"2"},
					"uint32_value": []string{"3"},
					"uint64_value": []string{"4"},
					"float_value":  []string{"5.5"},
					"double_value": []string{"6.6"},
					"bytes_value":  []string{"aGVsbG8"}, // base64URL
					"string_value": []string{"hello"},
					"field_mask":   []string{"user.displayName,photo"},
				}.Encode(),
			nil,
		),
		inouts: []any{
			in{
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
			out{
				msg: &emptypb.Empty{},
			},
		},
		want: want{
			statusCode: 200,
			msg:        &emptypb.Empty{},
		},
	}, {
		name: "complex",
		req: httptest.NewRequest(
			http.MethodGet,
			"/v1/complex?"+
				url.Values{
					"int32_value": []string{"1"},
					"int64_value": []string{"2"},

					// list values
					"int32_list": []string{"1", "2"},
					"string_list": []string{
						"hello",
						"world",
					},

					// enum values
					"enum_value": []string{"ENUM_VALUE"},

					// nested values
					"nested.int32_value": []string{"1"},
					"nested.enum_value":  []string{"ENUM_VALUE"},

					// oneof values
					"oneof_timestamp": []string{"2017-01-15T01:30:15.01Z"},
				}.Encode(),
			nil,
		),
		inouts: []any{
			in{
				method: "/larking.testpb.Complex/Check",
				msg: &testpb.ComplexRequest{
					Int32Value: 1,
					Int64Value: 2,
					Int32List: []int32{
						1, 2,
					},
					StringList: []string{
						"hello",
						"world",
					},
					EnumValue: testpb.ComplexRequest_ENUM_VALUE,
					Nested: &testpb.ComplexRequest_Nested{
						Int32Value: 1,
						EnumValue:  testpb.ComplexRequest_Nested_ENUM_VALUE,
					},
					Oneof: &testpb.ComplexRequest_OneofTimestamp{
						OneofTimestamp: &timestamppb.Timestamp{
							Seconds: 1484443815,
							Nanos:   10000000,
						},
					},
				},
			},
			out{
				msg: &emptypb.Empty{},
			},
		},
		want: want{
			statusCode: 200,
			msg:        &emptypb.Empty{},
		},
	}, {
		name: "complex-star",
		req: httptest.NewRequest(
			http.MethodGet,
			"/v1/complex/2.1/star/one",
			nil,
		),
		inouts: []any{
			in{
				method: "/larking.testpb.Complex/Check",
				msg: &testpb.ComplexRequest{
					DoubleValue: 2.1,
				},
			},
			out{
				msg: &emptypb.Empty{},
			},
		},
		want: want{
			statusCode: 200,
			msg:        &emptypb.Empty{},
		},
	}, {
		name: "complex-star/404",
		req: httptest.NewRequest(
			http.MethodGet,
			"/v1/complex/2.1/star/one/two/three",
			nil,
		),
		inouts: []any{
			in{
				method: "/larking.testpb.Complex/Check",
				msg: &testpb.ComplexRequest{
					DoubleValue: 2.1,
				},
			},
			out{},
		},
		want: want{
			statusCode: 404,
		},
	}, {
		name: "complex-starstar",
		req: httptest.NewRequest(
			http.MethodGet,
			"/v1/complex/2.1/starstar/one/two/three",
			nil,
		),
		inouts: []any{
			in{
				method: "/larking.testpb.Complex/Check",
				msg: &testpb.ComplexRequest{
					DoubleValue: 2.1,
				},
			},
			out{
				msg: &emptypb.Empty{},
			},
		},
		want: want{
			statusCode: 200,
			msg:        &emptypb.Empty{},
		},
	}, {
		name: "variable_one",
		req:  httptest.NewRequest(http.MethodGet, "/version/one", nil),
		inouts: []any{
			in{
				method: "/larking.testpb.Messaging/VariableOne",
				msg:    &testpb.Message{Text: "version"},
			},
			out{
				msg: &emptypb.Empty{},
			},
		},
		want: want{
			statusCode: 200,
			msg:        &emptypb.Empty{},
		},
	}, {
		name: "variable_two",
		req:  httptest.NewRequest(http.MethodGet, "/version/two", nil),
		inouts: []any{
			in{
				method: "/larking.testpb.Messaging/VariableTwo",
				msg:    &testpb.Message{Text: "version"},
			},
			out{
				msg: &emptypb.Empty{},
			},
		},
		want: want{
			statusCode: 200,
			msg:        &emptypb.Empty{},
		},
	}, {
		name: "shelf_name_get",
		req:  httptest.NewRequest(http.MethodGet, "/v1/shelves/shelf1", nil),
		inouts: []any{
			in{
				method: "/larking.testpb.Messaging/GetShelf",
				msg:    &testpb.GetShelfRequest{Name: "shelves/shelf1"},
			},
			out{
				msg: &testpb.Shelf{Name: "shelves/shelf1"},
			},
		},
		want: want{
			statusCode: 200,
			msg:        &testpb.Shelf{Name: "shelves/shelf1"},
		},
	}, {
		name: "book_name_get",
		req:  httptest.NewRequest(http.MethodGet, "/v1/shelves/shelf1/books/book2", nil),
		inouts: []any{
			in{
				method: "/larking.testpb.Messaging/GetBook",
				msg:    &testpb.GetBookRequest{Name: "shelves/shelf1/books/book2"},
			},
			out{
				msg: &testpb.Book{Name: "shelves/shelf1/books/book2"},
			},
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
		inouts: []any{
			in{
				method: "/larking.testpb.Messaging/CreateBook",
				msg: &testpb.CreateBookRequest{
					Parent: "shelves/shelf1",
					Book: &testpb.Book{
						Name: "book3",
					},
				},
			},
			out{
				msg: &testpb.Book{Name: "book3"},
			},
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
		inouts: []any{
			in{
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
			out{
				msg: &testpb.Book{
					Name:  "shelves/shelf1/books/book2",
					Title: "Lord of the Rings",
				},
			},
		},
		want: want{
			statusCode: 200,
			msg: &testpb.Book{
				Name:  "shelves/shelf1/books/book2",
				Title: "Lord of the Rings",
			},
		},
	}, {
		name: "book_name_get_implicit_GET",
		req: httptest.NewRequest(http.MethodGet, "/larking.testpb.Messaging/GetBook?"+url.Values{
			"name": []string{"shelves/shelf1/books/book2"},
		}.Encode(), nil),
		inouts: []any{
			in{
				method: "/larking.testpb.Messaging/GetBook",
				msg:    &testpb.GetBookRequest{Name: "shelves/shelf1/books/book2"},
			},
			out{
				msg: &testpb.Book{Name: "shelves/shelf1/books/book2"},
			},
		},
		want: want{
			statusCode: 200,
			msg:        &testpb.Book{Name: "shelves/shelf1/books/book2"},
		},
	}, {
		name: "book_name_get_implicit_POST",
		req: httptest.NewRequest(http.MethodPost, "/larking.testpb.Messaging/GetBook", strings.NewReader(
			`{ "name": "shelves/shelf1/books/book2" }`,
		)),
		inouts: []any{
			in{
				method: "/larking.testpb.Messaging/GetBook",
				msg:    &testpb.GetBookRequest{Name: "shelves/shelf1/books/book2"},
			},
			out{
				msg: &testpb.Book{Name: "shelves/shelf1/books/book2"},
			},
		},
		want: want{
			statusCode: 200,
			msg:        &testpb.Book{Name: "shelves/shelf1/books/book2"},
		},
	}}

	opts := cmp.Options{protocmp.Transform()}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o.reset(t, "test", tt.inouts)

			req := tt.req
			if len(tt.inouts) > 0 {
				req.Header["test"] = []string{tt.inouts[0].(in).method}
			}
			t.Log(req.Method, req.URL.String())

			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			resp := w.Result()

			t.Log(w.Body.String())

			b, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			t.Log(w.Code, w.Body.String())

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
				msg := tt.want.msg.ProtoReflect().New().Interface()
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

// TODO: one broken service breaks service descovery on reflection.
//// TestBrokenServer for error handling.
//func TestBrokenServer(t *testing.T) {
//	// Create test server.
//	bs := &testpb.UnimplementedBrokenServer{}
//
//	o := new(overrides)
//	gs := grpc.NewServer(o.unaryOption(), o.streamOption())
//
//	testpb.RegisterBrokenServer(gs, bs)
//
//	mux, err := NewMux()
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	// Call internal method of error handling.
//	err = mux.registerService(&testpb.Broken_ServiceDesc, &bs)
//	if err == nil {
//		t.Fatal("should fail")
//	}
//	t.Log(err)
//	if !strings.Contains(err.Error(), "invalid rule") {
//		t.Fatalf("unknown err: %v", err)
//	}
//}
