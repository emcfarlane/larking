/*
 *
 * Copyright 2016 gRPC authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

// Modified for ProtobufV2:
// https://raw.githubusercontent.com/grpc/grpc-go/v1.27.x/reflection/serverreflection.go
//
// Package reflection implements server reflection service.
//
// The service implemented is defined in:
// https://github.com/grpc/grpc/blob/master/src/proto/grpc/reflection/v1alpha/reflection.proto.
//
// To register server reflection on a gRPC server:
// 	import "google.golang.org/grpc/reflection"
//
// 	s := grpc.NewServer()
// 	pb.RegisterYourOwnServer(s, &server{})
//
// 	// Register reflection service on gRPC server.
// 	reflection.Register(s)
//
// 	s.Serve(lis)
//
package reflection

import (
	"fmt"
	"io"
	"sort"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	rpb "github.com/afking/graphpb/grpc/reflection/v1alpha"
)

type serverReflectionServer struct {
	s *grpc.Server

	initSymbols  sync.Once
	serviceNames []string
	symbols      map[protoreflect.FullName]protoreflect.FileDescriptor // map of fully-qualified names to files
}

// Register registers the server reflection service on the given gRPC server.
func Register(s *grpc.Server) {
	rpb.RegisterServerReflectionServer(s, &serverReflectionServer{
		s: s,
	})
}

func (s *serverReflectionServer) getSymbols() (svcNames []string, symbolIndex map[protoreflect.FullName]protoreflect.FileDescriptor) {
	s.initSymbols.Do(func() {
		serviceInfo := s.s.GetServiceInfo()

		s.symbols = make(map[protoreflect.FullName]protoreflect.FileDescriptor)
		s.serviceNames = make([]string, 0, len(serviceInfo))
		processed := make(map[string]bool)
		for svc, info := range serviceInfo {
			s.serviceNames = append(s.serviceNames, svc)

			file, ok := info.Metadata.(string)
			if !ok {
				panic(fmt.Sprintf("failed file %v", info))
				continue
			}

			fd, err := protoregistry.GlobalFiles.FindFileByPath(file)
			if err != nil {
				panic(err)
				continue
			}

			s.processFile(fd, processed)
		}
		sort.Strings(s.serviceNames)
	})
	return s.serviceNames, s.symbols
}

func (s *serverReflectionServer) processFile(fd protoreflect.FileDescriptor, processed map[string]bool) {
	filename := fd.Path()
	if processed[filename] {
		return
	}
	processed[filename] = true

	mds := fd.Messages()
	for i := 0; i < mds.Len(); i++ {
		s.processMessage(fd, mds.Get(i))
	}

	eds := fd.Enums()
	for i := 0; i < eds.Len(); i++ {
		s.processEnum(fd, eds.Get(i))
	}

	extds := fd.Extensions()
	for i := 0; i < extds.Len(); i++ {
		s.processField(fd, extds.Get(i))
	}

	sds := fd.Services()
	for i := 0; i < sds.Len(); i++ {
		sd := sds.Get(i)
		s.symbols[sd.FullName()] = fd

		mds := sd.Methods()
		for j := 0; j < mds.Len(); j++ {
			md := mds.Get(j)
			s.symbols[md.FullName()] = fd
		}
	}

	ids := fd.Imports()
	for i := 0; i < ids.Len(); i++ {
		ifd := ids.Get(i)
		s.processFile(ifd, processed)
	}
}

func (s *serverReflectionServer) processMessage(fd protoreflect.FileDescriptor, msg protoreflect.MessageDescriptor) {
	s.symbols[msg.FullName()] = fd

	mds := msg.Messages()
	for i := 0; i < mds.Len(); i++ {
		s.processMessage(fd, mds.Get(i))
	}

	eds := fd.Enums()
	for i := 0; i < eds.Len(); i++ {
		s.processEnum(fd, eds.Get(i))
	}

	extds := fd.Extensions()
	for i := 0; i < extds.Len(); i++ {
		s.processField(fd, extds.Get(i))
	}

	fds := msg.Fields()
	for i := 0; i < fds.Len(); i++ {
		s.processField(fd, fds.Get(i))
	}

	ods := msg.Oneofs()
	for i := 0; i < ods.Len(); i++ {
		s.symbols[ods.Get(i).FullName()] = fd
	}
}

func (s *serverReflectionServer) processEnum(fd protoreflect.FileDescriptor, en protoreflect.EnumDescriptor) {
	s.symbols[en.FullName()] = fd

	vs := en.Values()
	for i := 0; i < vs.Len(); i++ {
		s.symbols[vs.Get(i).FullName()] = fd
	}
}

func (s *serverReflectionServer) processField(fd protoreflect.FileDescriptor, fld protoreflect.FieldDescriptor) {
	s.symbols[fld.FullName()] = fd
}

// fileDescEncodingByFilename finds the file descriptor for given filename,
// does marshalling on it and returns the marshalled result.
func (s *serverReflectionServer) fileDescEncodingByFilename(name string) ([]byte, error) {
	fd, err := protoregistry.GlobalFiles.FindFileByPath(name)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(protodesc.ToFileDescriptorProto(fd))
}

// fileDescEncodingContainingSymbol finds the file descriptor containing the given symbol,
// does marshalling on it and returns the marshalled result.
// The given symbol can be a type, a service or a method.
func (s *serverReflectionServer) fileDescEncodingContainingSymbol(name string) ([]byte, error) {
	_, symbols := s.getSymbols()
	fd := symbols[protoreflect.FullName(name)]
	if fd == nil {
		// TODO: Check if it's a type name that was not present in the
		// transitive dependencies of the registered services.
		return nil, fmt.Errorf("unknown symbol: %v", name)
	}

	return proto.Marshal(protodesc.ToFileDescriptorProto(fd))
}

// fileDescEncodingContainingExtension finds the file descriptor containing given extension,
// does marshalling on it and returns the marshalled result.
func (s *serverReflectionServer) fileDescEncodingContainingExtension(typeName string, extNum int32) ([]byte, error) {
	typeFullName := protoreflect.FullName(typeName)
	ext, err := protoregistry.GlobalTypes.FindExtensionByName(typeFullName)
	if err != nil {
		return nil, err
	}

	// TODO: check this logic...
	extd := ext.TypeDescriptor()
	if int32(extd.Number()) != extNum {
		return nil, fmt.Errorf("failed to find registered extension for extension number %v", ext)
	}
	fd := extd.ParentFile()

	return proto.Marshal(protodesc.ToFileDescriptorProto(fd))
}

// allExtensionNumbersForTypeName returns all extension numbers for the given type.
func (s *serverReflectionServer) allExtensionNumbersForTypeName(name string) ([]int32, error) {
	fullname := protoreflect.FullName(name)
	md, err := protoregistry.GlobalTypes.FindMessageByName(fullname)
	if err != nil {
		return nil, err
	}

	extds := md.Descriptor().Extensions()
	extNums := make([]int32, extds.Len())
	for i := 0; i < extds.Len(); i++ {
		extd := extds.Get(i)
		extNums[i] = int32(extd.Number())
	}
	return extNums, nil
}

// ServerReflectionInfo is the reflection service handler.
func (s *serverReflectionServer) ServerReflectionInfo(stream rpb.ServerReflection_ServerReflectionInfoServer) error {
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		out := &rpb.ServerReflectionResponse{
			ValidHost:       in.Host,
			OriginalRequest: in,
		}
		switch req := in.MessageRequest.(type) {
		case *rpb.ServerReflectionRequest_FileByFilename:
			b, err := s.fileDescEncodingByFilename(req.FileByFilename)
			if err != nil {
				out.MessageResponse = &rpb.ServerReflectionResponse_ErrorResponse{
					ErrorResponse: &rpb.ErrorResponse{
						ErrorCode:    int32(codes.NotFound),
						ErrorMessage: err.Error(),
					},
				}
			} else {
				out.MessageResponse = &rpb.ServerReflectionResponse_FileDescriptorResponse{
					FileDescriptorResponse: &rpb.FileDescriptorResponse{FileDescriptorProto: [][]byte{b}},
				}
			}
		case *rpb.ServerReflectionRequest_FileContainingSymbol:
			b, err := s.fileDescEncodingContainingSymbol(req.FileContainingSymbol)
			if err != nil {
				out.MessageResponse = &rpb.ServerReflectionResponse_ErrorResponse{
					ErrorResponse: &rpb.ErrorResponse{
						ErrorCode:    int32(codes.NotFound),
						ErrorMessage: err.Error(),
					},
				}
			} else {
				out.MessageResponse = &rpb.ServerReflectionResponse_FileDescriptorResponse{
					FileDescriptorResponse: &rpb.FileDescriptorResponse{FileDescriptorProto: [][]byte{b}},
				}
			}
		case *rpb.ServerReflectionRequest_FileContainingExtension:
			typeName := req.FileContainingExtension.ContainingType
			extNum := req.FileContainingExtension.ExtensionNumber
			b, err := s.fileDescEncodingContainingExtension(typeName, extNum)
			if err != nil {
				out.MessageResponse = &rpb.ServerReflectionResponse_ErrorResponse{
					ErrorResponse: &rpb.ErrorResponse{
						ErrorCode:    int32(codes.NotFound),
						ErrorMessage: err.Error(),
					},
				}
			} else {
				out.MessageResponse = &rpb.ServerReflectionResponse_FileDescriptorResponse{
					FileDescriptorResponse: &rpb.FileDescriptorResponse{FileDescriptorProto: [][]byte{b}},
				}
			}
		case *rpb.ServerReflectionRequest_AllExtensionNumbersOfType:
			extNums, err := s.allExtensionNumbersForTypeName(req.AllExtensionNumbersOfType)
			if err != nil {
				out.MessageResponse = &rpb.ServerReflectionResponse_ErrorResponse{
					ErrorResponse: &rpb.ErrorResponse{
						ErrorCode:    int32(codes.NotFound),
						ErrorMessage: err.Error(),
					},
				}
			} else {
				out.MessageResponse = &rpb.ServerReflectionResponse_AllExtensionNumbersResponse{
					AllExtensionNumbersResponse: &rpb.ExtensionNumberResponse{
						BaseTypeName:    req.AllExtensionNumbersOfType,
						ExtensionNumber: extNums,
					},
				}
			}
		case *rpb.ServerReflectionRequest_ListServices:
			svcNames, _ := s.getSymbols()
			serviceResponses := make([]*rpb.ServiceResponse, len(svcNames))
			for i, n := range svcNames {
				serviceResponses[i] = &rpb.ServiceResponse{
					Name: n,
				}
			}
			out.MessageResponse = &rpb.ServerReflectionResponse_ListServicesResponse{
				ListServicesResponse: &rpb.ListServiceResponse{
					Service: serviceResponses,
				},
			}
		default:
			return status.Errorf(codes.InvalidArgument, "invalid MessageRequest: %v", in.MessageRequest)
		}

		if err := stream.Send(out); err != nil {
			return err
		}
	}
}
