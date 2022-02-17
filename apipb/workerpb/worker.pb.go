// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.27.1
// 	protoc        v3.19.4
// source: apipb/workerpb/worker.proto

package workerpb

import (
	_ "google.golang.org/genproto/googleapis/api/annotations"
	status "google.golang.org/genproto/googleapis/rpc/status"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type Command struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// Types that are assignable to Exec:
	//	*Command_Input
	//	*Command_Complete
	//	*Command_Format
	Exec isCommand_Exec `protobuf_oneof:"exec"`
}

func (x *Command) Reset() {
	*x = Command{}
	if protoimpl.UnsafeEnabled {
		mi := &file_apipb_workerpb_worker_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Command) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Command) ProtoMessage() {}

func (x *Command) ProtoReflect() protoreflect.Message {
	mi := &file_apipb_workerpb_worker_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Command.ProtoReflect.Descriptor instead.
func (*Command) Descriptor() ([]byte, []int) {
	return file_apipb_workerpb_worker_proto_rawDescGZIP(), []int{0}
}

func (x *Command) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (m *Command) GetExec() isCommand_Exec {
	if m != nil {
		return m.Exec
	}
	return nil
}

func (x *Command) GetInput() string {
	if x, ok := x.GetExec().(*Command_Input); ok {
		return x.Input
	}
	return ""
}

func (x *Command) GetComplete() string {
	if x, ok := x.GetExec().(*Command_Complete); ok {
		return x.Complete
	}
	return ""
}

func (x *Command) GetFormat() string {
	if x, ok := x.GetExec().(*Command_Format); ok {
		return x.Format
	}
	return ""
}

type isCommand_Exec interface {
	isCommand_Exec()
}

type Command_Input struct {
	Input string `protobuf:"bytes,2,opt,name=input,proto3,oneof"`
}

type Command_Complete struct {
	Complete string `protobuf:"bytes,3,opt,name=complete,proto3,oneof"`
}

type Command_Format struct {
	Format string `protobuf:"bytes,4,opt,name=format,proto3,oneof"`
}

func (*Command_Input) isCommand_Exec() {}

func (*Command_Complete) isCommand_Exec() {}

func (*Command_Format) isCommand_Exec() {}

type Output struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Output string         `protobuf:"bytes,1,opt,name=output,proto3" json:"output,omitempty"` // printed output
	Status *status.Status `protobuf:"bytes,2,opt,name=status,proto3" json:"status,omitempty"`
}

func (x *Output) Reset() {
	*x = Output{}
	if protoimpl.UnsafeEnabled {
		mi := &file_apipb_workerpb_worker_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Output) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Output) ProtoMessage() {}

func (x *Output) ProtoReflect() protoreflect.Message {
	mi := &file_apipb_workerpb_worker_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Output.ProtoReflect.Descriptor instead.
func (*Output) Descriptor() ([]byte, []int) {
	return file_apipb_workerpb_worker_proto_rawDescGZIP(), []int{1}
}

func (x *Output) GetOutput() string {
	if x != nil {
		return x.Output
	}
	return ""
}

func (x *Output) GetStatus() *status.Status {
	if x != nil {
		return x.Status
	}
	return nil
}

type Completion struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Completions []string `protobuf:"bytes,3,rep,name=completions,proto3" json:"completions,omitempty"` // completions
}

func (x *Completion) Reset() {
	*x = Completion{}
	if protoimpl.UnsafeEnabled {
		mi := &file_apipb_workerpb_worker_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Completion) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Completion) ProtoMessage() {}

func (x *Completion) ProtoReflect() protoreflect.Message {
	mi := &file_apipb_workerpb_worker_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Completion.ProtoReflect.Descriptor instead.
func (*Completion) Descriptor() ([]byte, []int) {
	return file_apipb_workerpb_worker_proto_rawDescGZIP(), []int{2}
}

func (x *Completion) GetCompletions() []string {
	if x != nil {
		return x.Completions
	}
	return nil
}

type Result struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Types that are assignable to Result:
	//	*Result_Output
	//	*Result_Completion
	Result isResult_Result `protobuf_oneof:"result"`
}

func (x *Result) Reset() {
	*x = Result{}
	if protoimpl.UnsafeEnabled {
		mi := &file_apipb_workerpb_worker_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Result) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Result) ProtoMessage() {}

func (x *Result) ProtoReflect() protoreflect.Message {
	mi := &file_apipb_workerpb_worker_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Result.ProtoReflect.Descriptor instead.
func (*Result) Descriptor() ([]byte, []int) {
	return file_apipb_workerpb_worker_proto_rawDescGZIP(), []int{3}
}

func (m *Result) GetResult() isResult_Result {
	if m != nil {
		return m.Result
	}
	return nil
}

func (x *Result) GetOutput() *Output {
	if x, ok := x.GetResult().(*Result_Output); ok {
		return x.Output
	}
	return nil
}

func (x *Result) GetCompletion() *Completion {
	if x, ok := x.GetResult().(*Result_Completion); ok {
		return x.Completion
	}
	return nil
}

type isResult_Result interface {
	isResult_Result()
}

type Result_Output struct {
	Output *Output `protobuf:"bytes,1,opt,name=output,proto3,oneof"`
}

type Result_Completion struct {
	Completion *Completion `protobuf:"bytes,2,opt,name=completion,proto3,oneof"`
}

func (*Result_Output) isResult_Result() {}

func (*Result_Completion) isResult_Result() {}

var File_apipb_workerpb_worker_proto protoreflect.FileDescriptor

var file_apipb_workerpb_worker_proto_rawDesc = []byte{
	0x0a, 0x1b, 0x61, 0x70, 0x69, 0x70, 0x62, 0x2f, 0x77, 0x6f, 0x72, 0x6b, 0x65, 0x72, 0x70, 0x62,
	0x2f, 0x77, 0x6f, 0x72, 0x6b, 0x65, 0x72, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x12, 0x6c,
	0x61, 0x72, 0x6b, 0x69, 0x6e, 0x67, 0x2e, 0x61, 0x70, 0x69, 0x2e, 0x77, 0x6f, 0x72, 0x6b, 0x65,
	0x72, 0x1a, 0x1c, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x61, 0x6e,
	0x6e, 0x6f, 0x74, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a,
	0x17, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x72, 0x70, 0x63, 0x2f, 0x73, 0x74, 0x61, 0x74,
	0x75, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x75, 0x0a, 0x07, 0x43, 0x6f, 0x6d, 0x6d,
	0x61, 0x6e, 0x64, 0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x16, 0x0a, 0x05, 0x69, 0x6e, 0x70, 0x75, 0x74,
	0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x48, 0x00, 0x52, 0x05, 0x69, 0x6e, 0x70, 0x75, 0x74, 0x12,
	0x1c, 0x0a, 0x08, 0x63, 0x6f, 0x6d, 0x70, 0x6c, 0x65, 0x74, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28,
	0x09, 0x48, 0x00, 0x52, 0x08, 0x63, 0x6f, 0x6d, 0x70, 0x6c, 0x65, 0x74, 0x65, 0x12, 0x18, 0x0a,
	0x06, 0x66, 0x6f, 0x72, 0x6d, 0x61, 0x74, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x48, 0x00, 0x52,
	0x06, 0x66, 0x6f, 0x72, 0x6d, 0x61, 0x74, 0x42, 0x06, 0x0a, 0x04, 0x65, 0x78, 0x65, 0x63, 0x22,
	0x4c, 0x0a, 0x06, 0x4f, 0x75, 0x74, 0x70, 0x75, 0x74, 0x12, 0x16, 0x0a, 0x06, 0x6f, 0x75, 0x74,
	0x70, 0x75, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x6f, 0x75, 0x74, 0x70, 0x75,
	0x74, 0x12, 0x2a, 0x0a, 0x06, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x0b, 0x32, 0x12, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x72, 0x70, 0x63, 0x2e, 0x53,
	0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x06, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x22, 0x2e, 0x0a,
	0x0a, 0x43, 0x6f, 0x6d, 0x70, 0x6c, 0x65, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x20, 0x0a, 0x0b, 0x63,
	0x6f, 0x6d, 0x70, 0x6c, 0x65, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x18, 0x03, 0x20, 0x03, 0x28, 0x09,
	0x52, 0x0b, 0x63, 0x6f, 0x6d, 0x70, 0x6c, 0x65, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x22, 0x8a, 0x01,
	0x0a, 0x06, 0x52, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x12, 0x34, 0x0a, 0x06, 0x6f, 0x75, 0x74, 0x70,
	0x75, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x6c, 0x61, 0x72, 0x6b, 0x69,
	0x6e, 0x67, 0x2e, 0x61, 0x70, 0x69, 0x2e, 0x77, 0x6f, 0x72, 0x6b, 0x65, 0x72, 0x2e, 0x4f, 0x75,
	0x74, 0x70, 0x75, 0x74, 0x48, 0x00, 0x52, 0x06, 0x6f, 0x75, 0x74, 0x70, 0x75, 0x74, 0x12, 0x40,
	0x0a, 0x0a, 0x63, 0x6f, 0x6d, 0x70, 0x6c, 0x65, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x02, 0x20, 0x01,
	0x28, 0x0b, 0x32, 0x1e, 0x2e, 0x6c, 0x61, 0x72, 0x6b, 0x69, 0x6e, 0x67, 0x2e, 0x61, 0x70, 0x69,
	0x2e, 0x77, 0x6f, 0x72, 0x6b, 0x65, 0x72, 0x2e, 0x43, 0x6f, 0x6d, 0x70, 0x6c, 0x65, 0x74, 0x69,
	0x6f, 0x6e, 0x48, 0x00, 0x52, 0x0a, 0x63, 0x6f, 0x6d, 0x70, 0x6c, 0x65, 0x74, 0x69, 0x6f, 0x6e,
	0x42, 0x08, 0x0a, 0x06, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x32, 0x86, 0x01, 0x0a, 0x06, 0x57,
	0x6f, 0x72, 0x6b, 0x65, 0x72, 0x12, 0x7c, 0x0a, 0x0b, 0x52, 0x75, 0x6e, 0x4f, 0x6e, 0x54, 0x68,
	0x72, 0x65, 0x61, 0x64, 0x12, 0x1b, 0x2e, 0x6c, 0x61, 0x72, 0x6b, 0x69, 0x6e, 0x67, 0x2e, 0x61,
	0x70, 0x69, 0x2e, 0x77, 0x6f, 0x72, 0x6b, 0x65, 0x72, 0x2e, 0x43, 0x6f, 0x6d, 0x6d, 0x61, 0x6e,
	0x64, 0x1a, 0x1a, 0x2e, 0x6c, 0x61, 0x72, 0x6b, 0x69, 0x6e, 0x67, 0x2e, 0x61, 0x70, 0x69, 0x2e,
	0x77, 0x6f, 0x72, 0x6b, 0x65, 0x72, 0x2e, 0x52, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x22, 0x30, 0x82,
	0xd3, 0xe4, 0x93, 0x02, 0x2a, 0x3a, 0x01, 0x2a, 0x42, 0x25, 0x0a, 0x09, 0x77, 0x65, 0x62, 0x73,
	0x6f, 0x63, 0x6b, 0x65, 0x74, 0x12, 0x18, 0x2f, 0x76, 0x31, 0x2f, 0x7b, 0x6e, 0x61, 0x6d, 0x65,
	0x3d, 0x74, 0x68, 0x72, 0x65, 0x61, 0x64, 0x73, 0x2f, 0x2a, 0x7d, 0x3a, 0x72, 0x75, 0x6e, 0x28,
	0x01, 0x30, 0x01, 0x42, 0x37, 0x5a, 0x35, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f,
	0x6d, 0x2f, 0x65, 0x6d, 0x63, 0x66, 0x61, 0x72, 0x6c, 0x61, 0x6e, 0x65, 0x2f, 0x6c, 0x61, 0x72,
	0x6b, 0x69, 0x6e, 0x67, 0x2f, 0x61, 0x70, 0x69, 0x70, 0x62, 0x2f, 0x77, 0x6f, 0x72, 0x6b, 0x65,
	0x72, 0x70, 0x62, 0x3b, 0x77, 0x6f, 0x72, 0x6b, 0x65, 0x72, 0x70, 0x62, 0x62, 0x06, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_apipb_workerpb_worker_proto_rawDescOnce sync.Once
	file_apipb_workerpb_worker_proto_rawDescData = file_apipb_workerpb_worker_proto_rawDesc
)

func file_apipb_workerpb_worker_proto_rawDescGZIP() []byte {
	file_apipb_workerpb_worker_proto_rawDescOnce.Do(func() {
		file_apipb_workerpb_worker_proto_rawDescData = protoimpl.X.CompressGZIP(file_apipb_workerpb_worker_proto_rawDescData)
	})
	return file_apipb_workerpb_worker_proto_rawDescData
}

var file_apipb_workerpb_worker_proto_msgTypes = make([]protoimpl.MessageInfo, 4)
var file_apipb_workerpb_worker_proto_goTypes = []interface{}{
	(*Command)(nil),       // 0: larking.api.worker.Command
	(*Output)(nil),        // 1: larking.api.worker.Output
	(*Completion)(nil),    // 2: larking.api.worker.Completion
	(*Result)(nil),        // 3: larking.api.worker.Result
	(*status.Status)(nil), // 4: google.rpc.Status
}
var file_apipb_workerpb_worker_proto_depIdxs = []int32{
	4, // 0: larking.api.worker.Output.status:type_name -> google.rpc.Status
	1, // 1: larking.api.worker.Result.output:type_name -> larking.api.worker.Output
	2, // 2: larking.api.worker.Result.completion:type_name -> larking.api.worker.Completion
	0, // 3: larking.api.worker.Worker.RunOnThread:input_type -> larking.api.worker.Command
	3, // 4: larking.api.worker.Worker.RunOnThread:output_type -> larking.api.worker.Result
	4, // [4:5] is the sub-list for method output_type
	3, // [3:4] is the sub-list for method input_type
	3, // [3:3] is the sub-list for extension type_name
	3, // [3:3] is the sub-list for extension extendee
	0, // [0:3] is the sub-list for field type_name
}

func init() { file_apipb_workerpb_worker_proto_init() }
func file_apipb_workerpb_worker_proto_init() {
	if File_apipb_workerpb_worker_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_apipb_workerpb_worker_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Command); i {
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
		file_apipb_workerpb_worker_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Output); i {
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
		file_apipb_workerpb_worker_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Completion); i {
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
		file_apipb_workerpb_worker_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Result); i {
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
	file_apipb_workerpb_worker_proto_msgTypes[0].OneofWrappers = []interface{}{
		(*Command_Input)(nil),
		(*Command_Complete)(nil),
		(*Command_Format)(nil),
	}
	file_apipb_workerpb_worker_proto_msgTypes[3].OneofWrappers = []interface{}{
		(*Result_Output)(nil),
		(*Result_Completion)(nil),
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_apipb_workerpb_worker_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   4,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_apipb_workerpb_worker_proto_goTypes,
		DependencyIndexes: file_apipb_workerpb_worker_proto_depIdxs,
		MessageInfos:      file_apipb_workerpb_worker_proto_msgTypes,
	}.Build()
	File_apipb_workerpb_worker_proto = out.File
	file_apipb_workerpb_worker_proto_rawDesc = nil
	file_apipb_workerpb_worker_proto_goTypes = nil
	file_apipb_workerpb_worker_proto_depIdxs = nil
}