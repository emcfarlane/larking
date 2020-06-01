package graphpb

import (
	"io"
	"sync"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"

	rpb "github.com/emcfarlane/graphpb/grpc/reflection/v1alpha"
)

func isStreamError(err error) bool {
	switch err {
	case nil:
		return false
	case io.EOF:
		return false
	case context.Canceled:
		return false
	}
	return true
}

// StreamHandler returns a gRPC stream handler to proxy gRPC requests.
func (m *Mux) StreamHandler() grpc.StreamHandler {
	return func(srv interface{}, stream grpc.ServerStream) error {
		ctx := stream.Context()
		name, _ := grpc.Method(ctx)
		s := m.loadState()

		mc, err := s.pickMethodConn(name)
		if err != nil {
			return err
		}

		// TODO: non marhsalling codec
		argsDesc := mc.desc.Input()
		replyDesc := mc.desc.Output()

		args := dynamicpb.NewMessage(argsDesc)
		reply := dynamicpb.NewMessage(replyDesc)

		if err := stream.RecvMsg(args); err != nil {
			return err
		}

		md, _ := metadata.FromIncomingContext(ctx)
		ctx = metadata.NewOutgoingContext(ctx, md)

		sd := &grpc.StreamDesc{
			ServerStreams: mc.desc.IsStreamingServer(),
			ClientStreams: mc.desc.IsStreamingClient(),
		}

		clientStream, err := mc.cc.NewStream(ctx, sd, name)
		if err != nil {
			return err
		}

		if err := clientStream.SendMsg(args); err != nil {
			return err
		}

		var inErr error
		var wg sync.WaitGroup
		if sd.ClientStreams {
			wg.Add(1)
			go func() {
				for {
					if inErr = stream.RecvMsg(args); inErr != nil {
						break
					}

					if inErr = clientStream.SendMsg(args); inErr != nil {
						break
					}
				}
				wg.Done()
			}()
		}

		var outErr error
		for {
			if outErr = clientStream.RecvMsg(reply); outErr != nil {
				break
			}

			if outErr = stream.SendMsg(reply); outErr != nil {
				break
			}

			if !sd.ServerStreams {
				break
			}
		}

		if isStreamError(outErr) {
			return outErr
		}

		if sd.ClientStreams {
			wg.Wait()
			if isStreamError(inErr) {
				return inErr
			}
		}

		trailer := clientStream.Trailer()
		stream.SetTrailer(trailer)

		return nil
	}
}

// TODO: fetch type on a per stream basis
type serverReflectionServer struct {
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
	fullname := protoreflect.FullName(name)
	d, err := protoregistry.GlobalFiles.FindDescriptorByName(fullname)
	if err != nil {
		return nil, err
	}
	fd := d.ParentFile()
	return proto.Marshal(protodesc.ToFileDescriptorProto(fd))
}

// fileDescEncodingContainingExtension finds the file descriptor containing given extension,
// does marshalling on it and returns the marshalled result.
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

// allExtensionNumbersForTypeName returns all extension numbers for the given type.
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
