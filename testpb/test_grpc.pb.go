// Code generated by protoc-gen-go-grpc. DO NOT EDIT.

package testpb

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConn

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion4

// MessagingClient is the client API for Messaging service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type MessagingClient interface {
	// HTTP | gRPC
	// -----|-----
	// `GET /v1/messages/123456`  | `GetMessageOne(name: "messages/123456")`
	GetMessageOne(ctx context.Context, in *GetMessageRequestOne, opts ...grpc.CallOption) (*Message, error)
	// HTTP | gRPC
	// -----|-----
	// `GET /v1/messages/123456?revision=2&sub.subfield=foo` |
	// `GetMessage(message_id: "123456" revision: 2 sub: SubMessage(subfield:
	// "foo"))`
	// `GET /v1/users/me/messages/123456` | `GetMessage(user_id: "me" message_id:
	// "123456")`
	GetMessageTwo(ctx context.Context, in *GetMessageRequestTwo, opts ...grpc.CallOption) (*Message, error)
	// HTTP | gRPC
	// -----|-----
	// `PATCH /v1/messages/123456 { "text": "Hi!" }` | `UpdateMessage(message_id:
	// "123456" message { text: "Hi!" })`
	UpdateMessage(ctx context.Context, in *UpdateMessageRequestOne, opts ...grpc.CallOption) (*Message, error)
	// HTTP | gRPC
	// -----|-----
	// `PATCH /v1/messages/123456 { "text": "Hi!" }` | `UpdateMessage(message_id:
	// "123456" text: "Hi!")`
	UpdateMessageBody(ctx context.Context, in *Message, opts ...grpc.CallOption) (*Message, error)
}

type messagingClient struct {
	cc *grpc.ClientConn
}

func NewMessagingClient(cc *grpc.ClientConn) MessagingClient {
	return &messagingClient{cc}
}

func (c *messagingClient) GetMessageOne(ctx context.Context, in *GetMessageRequestOne, opts ...grpc.CallOption) (*Message, error) {
	out := new(Message)
	err := c.cc.Invoke(ctx, "/gateway.testpb.Messaging/GetMessageOne", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *messagingClient) GetMessageTwo(ctx context.Context, in *GetMessageRequestTwo, opts ...grpc.CallOption) (*Message, error) {
	out := new(Message)
	err := c.cc.Invoke(ctx, "/gateway.testpb.Messaging/GetMessageTwo", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *messagingClient) UpdateMessage(ctx context.Context, in *UpdateMessageRequestOne, opts ...grpc.CallOption) (*Message, error) {
	out := new(Message)
	err := c.cc.Invoke(ctx, "/gateway.testpb.Messaging/UpdateMessage", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *messagingClient) UpdateMessageBody(ctx context.Context, in *Message, opts ...grpc.CallOption) (*Message, error) {
	out := new(Message)
	err := c.cc.Invoke(ctx, "/gateway.testpb.Messaging/UpdateMessageBody", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// MessagingServer is the server API for Messaging service.
type MessagingServer interface {
	// HTTP | gRPC
	// -----|-----
	// `GET /v1/messages/123456`  | `GetMessageOne(name: "messages/123456")`
	GetMessageOne(context.Context, *GetMessageRequestOne) (*Message, error)
	// HTTP | gRPC
	// -----|-----
	// `GET /v1/messages/123456?revision=2&sub.subfield=foo` |
	// `GetMessage(message_id: "123456" revision: 2 sub: SubMessage(subfield:
	// "foo"))`
	// `GET /v1/users/me/messages/123456` | `GetMessage(user_id: "me" message_id:
	// "123456")`
	GetMessageTwo(context.Context, *GetMessageRequestTwo) (*Message, error)
	// HTTP | gRPC
	// -----|-----
	// `PATCH /v1/messages/123456 { "text": "Hi!" }` | `UpdateMessage(message_id:
	// "123456" message { text: "Hi!" })`
	UpdateMessage(context.Context, *UpdateMessageRequestOne) (*Message, error)
	// HTTP | gRPC
	// -----|-----
	// `PATCH /v1/messages/123456 { "text": "Hi!" }` | `UpdateMessage(message_id:
	// "123456" text: "Hi!")`
	UpdateMessageBody(context.Context, *Message) (*Message, error)
}

// UnimplementedMessagingServer can be embedded to have forward compatible implementations.
type UnimplementedMessagingServer struct {
}

func (*UnimplementedMessagingServer) GetMessageOne(context.Context, *GetMessageRequestOne) (*Message, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetMessageOne not implemented")
}
func (*UnimplementedMessagingServer) GetMessageTwo(context.Context, *GetMessageRequestTwo) (*Message, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetMessageTwo not implemented")
}
func (*UnimplementedMessagingServer) UpdateMessage(context.Context, *UpdateMessageRequestOne) (*Message, error) {
	return nil, status.Errorf(codes.Unimplemented, "method UpdateMessage not implemented")
}
func (*UnimplementedMessagingServer) UpdateMessageBody(context.Context, *Message) (*Message, error) {
	return nil, status.Errorf(codes.Unimplemented, "method UpdateMessageBody not implemented")
}

func RegisterMessagingServer(s *grpc.Server, srv MessagingServer) {
	s.RegisterService(&_Messaging_serviceDesc, srv)
}

func _Messaging_GetMessageOne_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetMessageRequestOne)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(MessagingServer).GetMessageOne(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/gateway.testpb.Messaging/GetMessageOne",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(MessagingServer).GetMessageOne(ctx, req.(*GetMessageRequestOne))
	}
	return interceptor(ctx, in, info, handler)
}

func _Messaging_GetMessageTwo_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetMessageRequestTwo)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(MessagingServer).GetMessageTwo(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/gateway.testpb.Messaging/GetMessageTwo",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(MessagingServer).GetMessageTwo(ctx, req.(*GetMessageRequestTwo))
	}
	return interceptor(ctx, in, info, handler)
}

func _Messaging_UpdateMessage_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(UpdateMessageRequestOne)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(MessagingServer).UpdateMessage(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/gateway.testpb.Messaging/UpdateMessage",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(MessagingServer).UpdateMessage(ctx, req.(*UpdateMessageRequestOne))
	}
	return interceptor(ctx, in, info, handler)
}

func _Messaging_UpdateMessageBody_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(Message)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(MessagingServer).UpdateMessageBody(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/gateway.testpb.Messaging/UpdateMessageBody",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(MessagingServer).UpdateMessageBody(ctx, req.(*Message))
	}
	return interceptor(ctx, in, info, handler)
}

var _Messaging_serviceDesc = grpc.ServiceDesc{
	ServiceName: "gateway.testpb.Messaging",
	HandlerType: (*MessagingServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GetMessageOne",
			Handler:    _Messaging_GetMessageOne_Handler,
		},
		{
			MethodName: "GetMessageTwo",
			Handler:    _Messaging_GetMessageTwo_Handler,
		},
		{
			MethodName: "UpdateMessage",
			Handler:    _Messaging_UpdateMessage_Handler,
		},
		{
			MethodName: "UpdateMessageBody",
			Handler:    _Messaging_UpdateMessageBody_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "github.com/afking/gateway/testpb/test.proto",
}
