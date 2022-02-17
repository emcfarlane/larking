// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.26.0
// 	protoc        v3.19.3
// source: starlarkproto/testpb/star.proto

package testpb

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	descriptorpb "google.golang.org/protobuf/types/descriptorpb"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type Enum int32

const (
	Enum_ENUM_A Enum = 0
	Enum_ENUM_B Enum = 1
)

// Enum value maps for Enum.
var (
	Enum_name = map[int32]string{
		0: "ENUM_A",
		1: "ENUM_B",
	}
	Enum_value = map[string]int32{
		"ENUM_A": 0,
		"ENUM_B": 1,
	}
)

func (x Enum) Enum() *Enum {
	p := new(Enum)
	*p = x
	return p
}

func (x Enum) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (Enum) Descriptor() protoreflect.EnumDescriptor {
	return file_starlarkproto_testpb_star_proto_enumTypes[0].Descriptor()
}

func (Enum) Type() protoreflect.EnumType {
	return &file_starlarkproto_testpb_star_proto_enumTypes[0]
}

func (x Enum) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use Enum.Descriptor instead.
func (Enum) EnumDescriptor() ([]byte, []int) {
	return file_starlarkproto_testpb_star_proto_rawDescGZIP(), []int{0}
}

type Message_Type int32

const (
	Message_UNKNOWN  Message_Type = 0
	Message_GREETING Message_Type = 1
)

// Enum value maps for Message_Type.
var (
	Message_Type_name = map[int32]string{
		0: "UNKNOWN",
		1: "GREETING",
	}
	Message_Type_value = map[string]int32{
		"UNKNOWN":  0,
		"GREETING": 1,
	}
)

func (x Message_Type) Enum() *Message_Type {
	p := new(Message_Type)
	*p = x
	return p
}

func (x Message_Type) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (Message_Type) Descriptor() protoreflect.EnumDescriptor {
	return file_starlarkproto_testpb_star_proto_enumTypes[1].Descriptor()
}

func (Message_Type) Type() protoreflect.EnumType {
	return &file_starlarkproto_testpb_star_proto_enumTypes[1]
}

func (x Message_Type) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use Message_Type.Descriptor instead.
func (Message_Type) EnumDescriptor() ([]byte, []int) {
	return file_starlarkproto_testpb_star_proto_rawDescGZIP(), []int{0, 0}
}

type Message struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Body    string              `protobuf:"bytes,1,opt,name=body,proto3" json:"body,omitempty"`
	Type    Message_Type        `protobuf:"varint,2,opt,name=type,proto3,enum=starlarkproto.test.Message_Type" json:"type,omitempty"`
	Strings []string            `protobuf:"bytes,3,rep,name=strings,proto3" json:"strings,omitempty"`
	Nested  *Message            `protobuf:"bytes,4,opt,name=nested,proto3" json:"nested,omitempty"`
	Maps    map[string]*Message `protobuf:"bytes,5,rep,name=maps,proto3" json:"maps,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
	// Types that are assignable to Oneofs:
	//	*Message_OneString
	//	*Message_OneNumber
	Oneofs isMessage_Oneofs `protobuf_oneof:"oneofs"`
}

func (x *Message) Reset() {
	*x = Message{}
	if protoimpl.UnsafeEnabled {
		mi := &file_starlarkproto_testpb_star_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Message) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Message) ProtoMessage() {}

func (x *Message) ProtoReflect() protoreflect.Message {
	mi := &file_starlarkproto_testpb_star_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Message.ProtoReflect.Descriptor instead.
func (*Message) Descriptor() ([]byte, []int) {
	return file_starlarkproto_testpb_star_proto_rawDescGZIP(), []int{0}
}

func (x *Message) GetBody() string {
	if x != nil {
		return x.Body
	}
	return ""
}

func (x *Message) GetType() Message_Type {
	if x != nil {
		return x.Type
	}
	return Message_UNKNOWN
}

func (x *Message) GetStrings() []string {
	if x != nil {
		return x.Strings
	}
	return nil
}

func (x *Message) GetNested() *Message {
	if x != nil {
		return x.Nested
	}
	return nil
}

func (x *Message) GetMaps() map[string]*Message {
	if x != nil {
		return x.Maps
	}
	return nil
}

func (m *Message) GetOneofs() isMessage_Oneofs {
	if m != nil {
		return m.Oneofs
	}
	return nil
}

func (x *Message) GetOneString() string {
	if x, ok := x.GetOneofs().(*Message_OneString); ok {
		return x.OneString
	}
	return ""
}

func (x *Message) GetOneNumber() int64 {
	if x, ok := x.GetOneofs().(*Message_OneNumber); ok {
		return x.OneNumber
	}
	return 0
}

type isMessage_Oneofs interface {
	isMessage_Oneofs()
}

type Message_OneString struct {
	OneString string `protobuf:"bytes,6,opt,name=one_string,json=oneString,proto3,oneof"`
}

type Message_OneNumber struct {
	OneNumber int64 `protobuf:"varint,7,opt,name=one_number,json=oneNumber,proto3,oneof"`
}

func (*Message_OneString) isMessage_Oneofs() {}

func (*Message_OneNumber) isMessage_Oneofs() {}

type GetMessageRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	MessageId string `protobuf:"bytes,1,opt,name=message_id,json=messageId,proto3" json:"message_id,omitempty"`
	UserId    string `protobuf:"bytes,2,opt,name=user_id,json=userId,proto3" json:"user_id,omitempty"`
}

func (x *GetMessageRequest) Reset() {
	*x = GetMessageRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_starlarkproto_testpb_star_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *GetMessageRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetMessageRequest) ProtoMessage() {}

func (x *GetMessageRequest) ProtoReflect() protoreflect.Message {
	mi := &file_starlarkproto_testpb_star_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetMessageRequest.ProtoReflect.Descriptor instead.
func (*GetMessageRequest) Descriptor() ([]byte, []int) {
	return file_starlarkproto_testpb_star_proto_rawDescGZIP(), []int{1}
}

func (x *GetMessageRequest) GetMessageId() string {
	if x != nil {
		return x.MessageId
	}
	return ""
}

func (x *GetMessageRequest) GetUserId() string {
	if x != nil {
		return x.UserId
	}
	return ""
}

var file_starlarkproto_testpb_star_proto_extTypes = []protoimpl.ExtensionInfo{
	{
		ExtendedType:  (*descriptorpb.MessageOptions)(nil),
		ExtensionType: (*string)(nil),
		Field:         51234,
		Name:          "starlarkproto.test.my_option",
		Tag:           "bytes,51234,opt,name=my_option",
		Filename:      "starlarkproto/testpb/star.proto",
	},
	{
		ExtendedType:  (*descriptorpb.MethodOptions)(nil),
		ExtensionType: ([]string)(nil),
		Field:         88888888,
		Name:          "starlarkproto.test.tag",
		Tag:           "bytes,88888888,rep,name=tag",
		Filename:      "starlarkproto/testpb/star.proto",
	},
}

// Extension fields to descriptorpb.MessageOptions.
var (
	// optional string my_option = 51234;
	E_MyOption = &file_starlarkproto_testpb_star_proto_extTypes[0]
)

// Extension fields to descriptorpb.MethodOptions.
var (
	// repeated string tag = 88888888;
	E_Tag = &file_starlarkproto_testpb_star_proto_extTypes[1]
)

var File_starlarkproto_testpb_star_proto protoreflect.FileDescriptor

var file_starlarkproto_testpb_star_proto_rawDesc = []byte{
	0x0a, 0x1f, 0x73, 0x74, 0x61, 0x72, 0x6c, 0x61, 0x72, 0x6b, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f,
	0x74, 0x65, 0x73, 0x74, 0x70, 0x62, 0x2f, 0x73, 0x74, 0x61, 0x72, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x12, 0x12, 0x73, 0x74, 0x61, 0x72, 0x6c, 0x61, 0x72, 0x6b, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x2e, 0x74, 0x65, 0x73, 0x74, 0x1a, 0x20, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x6f,
	0x72, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0xb4, 0x03, 0x0a, 0x07, 0x4d, 0x65, 0x73, 0x73,
	0x61, 0x67, 0x65, 0x12, 0x12, 0x0a, 0x04, 0x62, 0x6f, 0x64, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x04, 0x62, 0x6f, 0x64, 0x79, 0x12, 0x34, 0x0a, 0x04, 0x74, 0x79, 0x70, 0x65, 0x18,
	0x02, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x20, 0x2e, 0x73, 0x74, 0x61, 0x72, 0x6c, 0x61, 0x72, 0x6b,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2e, 0x74, 0x65, 0x73, 0x74, 0x2e, 0x4d, 0x65, 0x73, 0x73, 0x61,
	0x67, 0x65, 0x2e, 0x54, 0x79, 0x70, 0x65, 0x52, 0x04, 0x74, 0x79, 0x70, 0x65, 0x12, 0x18, 0x0a,
	0x07, 0x73, 0x74, 0x72, 0x69, 0x6e, 0x67, 0x73, 0x18, 0x03, 0x20, 0x03, 0x28, 0x09, 0x52, 0x07,
	0x73, 0x74, 0x72, 0x69, 0x6e, 0x67, 0x73, 0x12, 0x33, 0x0a, 0x06, 0x6e, 0x65, 0x73, 0x74, 0x65,
	0x64, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1b, 0x2e, 0x73, 0x74, 0x61, 0x72, 0x6c, 0x61,
	0x72, 0x6b, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2e, 0x74, 0x65, 0x73, 0x74, 0x2e, 0x4d, 0x65, 0x73,
	0x73, 0x61, 0x67, 0x65, 0x52, 0x06, 0x6e, 0x65, 0x73, 0x74, 0x65, 0x64, 0x12, 0x39, 0x0a, 0x04,
	0x6d, 0x61, 0x70, 0x73, 0x18, 0x05, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x25, 0x2e, 0x73, 0x74, 0x61,
	0x72, 0x6c, 0x61, 0x72, 0x6b, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2e, 0x74, 0x65, 0x73, 0x74, 0x2e,
	0x4d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x2e, 0x4d, 0x61, 0x70, 0x73, 0x45, 0x6e, 0x74, 0x72,
	0x79, 0x52, 0x04, 0x6d, 0x61, 0x70, 0x73, 0x12, 0x1f, 0x0a, 0x0a, 0x6f, 0x6e, 0x65, 0x5f, 0x73,
	0x74, 0x72, 0x69, 0x6e, 0x67, 0x18, 0x06, 0x20, 0x01, 0x28, 0x09, 0x48, 0x00, 0x52, 0x09, 0x6f,
	0x6e, 0x65, 0x53, 0x74, 0x72, 0x69, 0x6e, 0x67, 0x12, 0x1f, 0x0a, 0x0a, 0x6f, 0x6e, 0x65, 0x5f,
	0x6e, 0x75, 0x6d, 0x62, 0x65, 0x72, 0x18, 0x07, 0x20, 0x01, 0x28, 0x03, 0x48, 0x00, 0x52, 0x09,
	0x6f, 0x6e, 0x65, 0x4e, 0x75, 0x6d, 0x62, 0x65, 0x72, 0x1a, 0x54, 0x0a, 0x09, 0x4d, 0x61, 0x70,
	0x73, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x12, 0x10, 0x0a, 0x03, 0x6b, 0x65, 0x79, 0x18, 0x01, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x03, 0x6b, 0x65, 0x79, 0x12, 0x31, 0x0a, 0x05, 0x76, 0x61, 0x6c, 0x75,
	0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1b, 0x2e, 0x73, 0x74, 0x61, 0x72, 0x6c, 0x61,
	0x72, 0x6b, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2e, 0x74, 0x65, 0x73, 0x74, 0x2e, 0x4d, 0x65, 0x73,
	0x73, 0x61, 0x67, 0x65, 0x52, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x3a, 0x02, 0x38, 0x01, 0x22,
	0x21, 0x0a, 0x04, 0x54, 0x79, 0x70, 0x65, 0x12, 0x0b, 0x0a, 0x07, 0x55, 0x4e, 0x4b, 0x4e, 0x4f,
	0x57, 0x4e, 0x10, 0x00, 0x12, 0x0c, 0x0a, 0x08, 0x47, 0x52, 0x45, 0x45, 0x54, 0x49, 0x4e, 0x47,
	0x10, 0x01, 0x3a, 0x10, 0x92, 0x82, 0x19, 0x0c, 0x48, 0x65, 0x6c, 0x6c, 0x6f, 0x20, 0x77, 0x6f,
	0x72, 0x6c, 0x64, 0x21, 0x42, 0x08, 0x0a, 0x06, 0x6f, 0x6e, 0x65, 0x6f, 0x66, 0x73, 0x22, 0x4b,
	0x0a, 0x11, 0x47, 0x65, 0x74, 0x4d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x52, 0x65, 0x71, 0x75,
	0x65, 0x73, 0x74, 0x12, 0x1d, 0x0a, 0x0a, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x5f, 0x69,
	0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x09, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65,
	0x49, 0x64, 0x12, 0x17, 0x0a, 0x07, 0x75, 0x73, 0x65, 0x72, 0x5f, 0x69, 0x64, 0x18, 0x02, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x06, 0x75, 0x73, 0x65, 0x72, 0x49, 0x64, 0x2a, 0x1e, 0x0a, 0x04, 0x45,
	0x6e, 0x75, 0x6d, 0x12, 0x0a, 0x0a, 0x06, 0x45, 0x4e, 0x55, 0x4d, 0x5f, 0x41, 0x10, 0x00, 0x12,
	0x0a, 0x0a, 0x06, 0x45, 0x4e, 0x55, 0x4d, 0x5f, 0x42, 0x10, 0x01, 0x32, 0x6b, 0x0a, 0x09, 0x4d,
	0x65, 0x73, 0x73, 0x61, 0x67, 0x69, 0x6e, 0x67, 0x12, 0x5e, 0x0a, 0x0a, 0x47, 0x65, 0x74, 0x4d,
	0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x12, 0x25, 0x2e, 0x73, 0x74, 0x61, 0x72, 0x6c, 0x61, 0x72,
	0x6b, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2e, 0x74, 0x65, 0x73, 0x74, 0x2e, 0x47, 0x65, 0x74, 0x4d,
	0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x1b, 0x2e,
	0x73, 0x74, 0x61, 0x72, 0x6c, 0x61, 0x72, 0x6b, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2e, 0x74, 0x65,
	0x73, 0x74, 0x2e, 0x4d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x22, 0x0c, 0xc2, 0xe3, 0x8a, 0xd3,
	0x02, 0x06, 0x74, 0x61, 0x67, 0x67, 0x65, 0x64, 0x3a, 0x3e, 0x0a, 0x09, 0x6d, 0x79, 0x5f, 0x6f,
	0x70, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x1f, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x4d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x4f,
	0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x18, 0xa2, 0x90, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08,
	0x6d, 0x79, 0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x3a, 0x33, 0x0a, 0x03, 0x74, 0x61, 0x67, 0x12,
	0x1e, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75,
	0x66, 0x2e, 0x4d, 0x65, 0x74, 0x68, 0x6f, 0x64, 0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x18,
	0xb8, 0xac, 0xb1, 0x2a, 0x20, 0x03, 0x28, 0x09, 0x52, 0x03, 0x74, 0x61, 0x67, 0x42, 0x34, 0x5a,
	0x32, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x65, 0x6d, 0x63, 0x66,
	0x61, 0x72, 0x6c, 0x61, 0x6e, 0x65, 0x2f, 0x6c, 0x61, 0x72, 0x6b, 0x69, 0x6e, 0x67, 0x2f, 0x73,
	0x74, 0x61, 0x72, 0x6c, 0x61, 0x72, 0x6b, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x74, 0x65, 0x73,
	0x74, 0x70, 0x62, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_starlarkproto_testpb_star_proto_rawDescOnce sync.Once
	file_starlarkproto_testpb_star_proto_rawDescData = file_starlarkproto_testpb_star_proto_rawDesc
)

func file_starlarkproto_testpb_star_proto_rawDescGZIP() []byte {
	file_starlarkproto_testpb_star_proto_rawDescOnce.Do(func() {
		file_starlarkproto_testpb_star_proto_rawDescData = protoimpl.X.CompressGZIP(file_starlarkproto_testpb_star_proto_rawDescData)
	})
	return file_starlarkproto_testpb_star_proto_rawDescData
}

var file_starlarkproto_testpb_star_proto_enumTypes = make([]protoimpl.EnumInfo, 2)
var file_starlarkproto_testpb_star_proto_msgTypes = make([]protoimpl.MessageInfo, 3)
var file_starlarkproto_testpb_star_proto_goTypes = []interface{}{
	(Enum)(0),                           // 0: starlarkproto.test.Enum
	(Message_Type)(0),                   // 1: starlarkproto.test.Message.Type
	(*Message)(nil),                     // 2: starlarkproto.test.Message
	(*GetMessageRequest)(nil),           // 3: starlarkproto.test.GetMessageRequest
	nil,                                 // 4: starlarkproto.test.Message.MapsEntry
	(*descriptorpb.MessageOptions)(nil), // 5: google.protobuf.MessageOptions
	(*descriptorpb.MethodOptions)(nil),  // 6: google.protobuf.MethodOptions
}
var file_starlarkproto_testpb_star_proto_depIdxs = []int32{
	1, // 0: starlarkproto.test.Message.type:type_name -> starlarkproto.test.Message.Type
	2, // 1: starlarkproto.test.Message.nested:type_name -> starlarkproto.test.Message
	4, // 2: starlarkproto.test.Message.maps:type_name -> starlarkproto.test.Message.MapsEntry
	2, // 3: starlarkproto.test.Message.MapsEntry.value:type_name -> starlarkproto.test.Message
	5, // 4: starlarkproto.test.my_option:extendee -> google.protobuf.MessageOptions
	6, // 5: starlarkproto.test.tag:extendee -> google.protobuf.MethodOptions
	3, // 6: starlarkproto.test.Messaging.GetMessage:input_type -> starlarkproto.test.GetMessageRequest
	2, // 7: starlarkproto.test.Messaging.GetMessage:output_type -> starlarkproto.test.Message
	7, // [7:8] is the sub-list for method output_type
	6, // [6:7] is the sub-list for method input_type
	6, // [6:6] is the sub-list for extension type_name
	4, // [4:6] is the sub-list for extension extendee
	0, // [0:4] is the sub-list for field type_name
}

func init() { file_starlarkproto_testpb_star_proto_init() }
func file_starlarkproto_testpb_star_proto_init() {
	if File_starlarkproto_testpb_star_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_starlarkproto_testpb_star_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Message); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_starlarkproto_testpb_star_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*GetMessageRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	file_starlarkproto_testpb_star_proto_msgTypes[0].OneofWrappers = []interface{}{
		(*Message_OneString)(nil),
		(*Message_OneNumber)(nil),
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_starlarkproto_testpb_star_proto_rawDesc,
			NumEnums:      2,
			NumMessages:   3,
			NumExtensions: 2,
			NumServices:   1,
		},
		GoTypes:           file_starlarkproto_testpb_star_proto_goTypes,
		DependencyIndexes: file_starlarkproto_testpb_star_proto_depIdxs,
		EnumInfos:         file_starlarkproto_testpb_star_proto_enumTypes,
		MessageInfos:      file_starlarkproto_testpb_star_proto_msgTypes,
		ExtensionInfos:    file_starlarkproto_testpb_star_proto_extTypes,
	}.Build()
	File_starlarkproto_testpb_star_proto = out.File
	file_starlarkproto_testpb_star_proto_rawDesc = nil
	file_starlarkproto_testpb_star_proto_goTypes = nil
	file_starlarkproto_testpb_star_proto_depIdxs = nil
}