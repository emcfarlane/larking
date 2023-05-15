// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package larking

import (
	"io"

	"google.golang.org/grpc"
	rpb "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// TODO: fetch type on a per stream basis
type serverReflectionServer struct {
	rpb.UnimplementedServerReflectionServer
	m *Mux
	s *grpc.Server
}

// RegisterReflectionServer registers the server reflection service for multiple
// proxied gRPC servers. Each individual reflection stream is merged to provide
// a consistent view at the point of stream creation.
func (m *Mux) RegisterReflectionServer(s *grpc.Server) {
	rpb.RegisterServerReflectionServer(s, &serverReflectionServer{
		m: m,
		s: s,
	})
}

// does marshalling on it and returns the marshalled result.
//
//nolint:unused // fileDescEncodingByFilename finds the file descriptor for given filename,
func (s *serverReflectionServer) fileDescEncodingByFilename(name string) ([]byte, error) {
	fd, err := protoregistry.GlobalFiles.FindFileByPath(name)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(protodesc.ToFileDescriptorProto(fd))
}

// does marshalling on it and returns the marshalled result.
// The given symbol can be a type, a service or a method.
//
//nolint:unused // fileDescEncodingContainingSymbol finds the file descriptor containing the given symbol,
func (s *serverReflectionServer) fileDescEncodingContainingSymbol(name string) ([]byte, error) {
	fullname := protoreflect.FullName(name)
	d, err := protoregistry.GlobalFiles.FindDescriptorByName(fullname)
	if err != nil {
		return nil, err
	}
	fd := d.ParentFile()
	return proto.Marshal(protodesc.ToFileDescriptorProto(fd))
}

// does marshalling on it and returns the marshalled result.
//
//nolint:unused // fileDescEncodingContainingExtension finds the file descriptor containing given extension,
func (s *serverReflectionServer) fileDescEncodingContainingExtension(typeName string, extNum int32) ([]byte, error) {
	fullname := protoreflect.FullName(typeName)
	fieldnumber := protoreflect.FieldNumber(extNum)
	ext, err := protoregistry.GlobalTypes.FindExtensionByNumber(fullname, fieldnumber)
	if err != nil {
		return nil, err
	}

	extd := ext.TypeDescriptor()
	d, err := protoregistry.GlobalFiles.FindDescriptorByName(extd.FullName())
	if err != nil {
		return nil, err
	}
	fd := d.ParentFile()

	return proto.Marshal(protodesc.ToFileDescriptorProto(fd))
}

//nolint:unused // allExtensionNumbersForTypeName returns all extension numbers for the given type.
func (s *serverReflectionServer) allExtensionNumbersForTypeName(name string) ([]int32, error) {
	fullname := protoreflect.FullName(name)
	_, err := protoregistry.GlobalFiles.FindDescriptorByName(fullname)
	if err != nil {
		return nil, err
	}

	n := protoregistry.GlobalTypes.NumExtensionsByMessage(fullname)
	if n == 0 {
		return nil, nil
	}

	extNums := make([]int32, 0, n)
	protoregistry.GlobalTypes.RangeExtensionsByMessage(
		fullname,
		func(et protoreflect.ExtensionType) bool {
			ed := et.TypeDescriptor().Descriptor()
			extNums = append(extNums, int32(ed.Number()))
			return true
		},
	)
	return extNums, nil
}

// ServerReflectionInfo is the reflection service handler.
func (s *serverReflectionServer) ServerReflectionInfo(stream rpb.ServerReflection_ServerReflectionInfoServer) error {

	//ss := s.m.loadState()

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
		/*switch req := in.MessageRequest.(type) {
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
			svcInfo := s.s.GetServiceInfo()
			serviceResponses := make([]*rpb.ServiceResponse, 0, len(svcInfo))
			for svcName := range svcInfo {
				serviceResponses = append(serviceResponses, &rpb.ServiceResponse{
					Name: svcName,
				})
			}
			sort.Slice(serviceResponses, func(i, j int) bool {
				return serviceResponses[i].Name < serviceResponses[j].Name
			})
			out.MessageResponse = &rpb.ServerReflectionResponse_ListServicesResponse{
				ListServicesResponse: &rpb.ListServiceResponse{
					Service: serviceResponses,
				},
			}
		default:
			return status.Errorf(codes.InvalidArgument, "invalid MessageRequest: %v", in.MessageRequest)
		}*/

		if err := stream.Send(out); err != nil {
			return err
		}
	}
}
