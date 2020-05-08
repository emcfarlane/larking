package graphpb

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/afking/graphpb/google.golang.org/genproto/googleapis/api/annotations"
	"github.com/afking/graphpb/google.golang.org/genproto/googleapis/api/httpbody"
	"github.com/afking/graphpb/grpc/codes"
	rpb "github.com/afking/graphpb/grpc/reflection/v1alpha"
	"github.com/afking/graphpb/grpc/status"
)

type methodDesc struct {
	name string
	desc protoreflect.MethodDescriptor
}

type methodConn struct {
	methodDesc
	cc *grpc.ClientConn
}

type Mux struct {
	processed map[protoreflect.FullName]bool
	path      *path

	// TODO: copy on write?
	mu      sync.Mutex
	conns   map[*grpc.ClientConn][]methodDesc
	methods map[string][]methodConn //*grpc.ClientConn
	//methodDescs map[string]protoreflect.MethodDescriptor
}

func NewMux(ccs ...*grpc.ClientConn) (*Mux, error) {
	m := &Mux{
		processed: make(map[protoreflect.FullName]bool),
		path:      newPath(),
		conns:     make(map[*grpc.ClientConn][]methodDesc),
		methods:   make(map[string][]methodConn), //*grpc.ClientConn),
	}

	for _, cc := range ccs {
		if err := m.createHandler(cc); err != nil {
			return nil, err
		}
	}
	return m, nil
}

// resolver implements protodesc.Resolver.
type resolver struct {
	files  protoregistry.Files
	stream rpb.ServerReflection_ServerReflectionInfoClient
}

func newResolver(stream rpb.ServerReflection_ServerReflectionInfoClient) (*resolver, error) {
	r := &resolver{stream: stream}

	if err := r.files.RegisterFile(annotations.File_google_api_annotations_proto); err != nil {
		return nil, err
	}
	if err := r.files.RegisterFile(annotations.File_google_api_http_proto); err != nil {
		return nil, err
	}
	if err := r.files.RegisterFile(httpbody.File_google_api_httpbody_proto); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *resolver) FindFileByPath(path string) (protoreflect.FileDescriptor, error) {
	if fd, err := r.files.FindFileByPath(path); err == nil {
		return fd, nil // found file
	}

	// TODO: locking?
	if err := r.stream.Send(&rpb.ServerReflectionRequest{
		MessageRequest: &rpb.ServerReflectionRequest_FileByFilename{
			FileByFilename: path,
		},
	}); err != nil {
		return nil, err
	}

	fdr, err := r.stream.Recv()
	if err != nil {
		return nil, err
	}
	fdbs := fdr.GetFileDescriptorResponse().GetFileDescriptorProto()

	var f protoreflect.FileDescriptor
	for _, fdb := range fdbs {
		fdp := &descriptorpb.FileDescriptorProto{}
		if err := proto.Unmarshal(fdb, fdp); err != nil {
			return nil, err
		}

		file, err := protodesc.NewFile(fdp, r)
		if err != nil {
			return nil, err
		}
		// TODO: check duplicate file registry
		if err := r.files.RegisterFile(file); err != nil {
			return nil, err
		}
		if file.Path() == path {
			f = file
		}
	}
	if f == nil {
		return nil, fmt.Errorf("missing file descriptor %s", path)
	}
	return f, nil
}

func (r *resolver) FindDescriptorByName(fullname protoreflect.FullName) (protoreflect.Descriptor, error) {
	return r.files.FindDescriptorByName(fullname)
}

func (m *Mux) createHandler(cc *grpc.ClientConn) error {
	// TODO: async fetch and mux creation.

	c := rpb.NewServerReflectionClient(cc)

	// TODO: watch the stream. When it is recreated refresh the service
	// methods and recreate the mux if needed.
	stream, err := c.ServerReflectionInfo(context.Background(), grpc.WaitForReady(true))
	if err != nil {
		return err
	}

	if err := stream.Send(&rpb.ServerReflectionRequest{
		MessageRequest: &rpb.ServerReflectionRequest_ListServices{},
	}); err != nil {
		return err
	}

	r, err := stream.Recv()
	if err != nil {
		return err
	}
	// TODO: check r.GetErrorResponse()?

	fds := make(map[string]*descriptorpb.FileDescriptorProto)
	for _, svc := range r.GetListServicesResponse().GetService() {
		fmt.Println("GOT SERVICES!", svc)
		if err := stream.Send(&rpb.ServerReflectionRequest{
			MessageRequest: &rpb.ServerReflectionRequest_FileContainingSymbol{
				FileContainingSymbol: svc.GetName(),
			},
		}); err != nil {
			return err
		}

		fdr, err := stream.Recv()
		if err != nil {
			return err
		}

		fdbb := fdr.GetFileDescriptorResponse().GetFileDescriptorProto()

		for _, fdb := range fdbb {
			fd := &descriptorpb.FileDescriptorProto{}
			if err := proto.Unmarshal(fdb, fd); err != nil {
				return err
			}
			fds[fd.GetName()] = fd
		}
	}

	rslvr, err := newResolver(stream)
	if err != nil {
		return err
	}

	var methods []methodDesc
	for _, fd := range fds {
		file, err := protodesc.NewFile(fd, rslvr)
		if err != nil {
			return err
		}

		ms, err := m.processFile(cc, file)
		if err != nil {
			// TODO: partial dregister
			return err
		}
		methods = append(methods, ms...)
	}

	// Update
	m.mu.Lock()
	m.conns[cc] = methods
	for _, method := range methods {
		m.methods[method.name] = append(
			m.methods[method.name], methodConn{method, cc},
		)
	}
	m.mu.Unlock()
	return nil
}

func (m *Mux) processFile(cc *grpc.ClientConn, fd protoreflect.FileDescriptor) ([]methodDesc, error) {
	//fmt.Println("processFile", fd.Name())
	var methods []methodDesc

	sds := fd.Services()
	for i := 0; i < sds.Len(); i++ {
		sd := sds.Get(i)
		name := sd.FullName()

		mds := sd.Methods()
		for j := 0; j < mds.Len(); j++ {
			md := mds.Get(j)

			opts := md.Options() // TODO: nil check fails?

			rule := getExtensionHTTP(opts)
			if rule == nil {
				continue
			}

			method := fmt.Sprintf("/%s/%s", name, md.Name())
			if err := m.path.parseRule(rule, md, method); err != nil {
				// TODO: partial service registration?
				return nil, err
			}
			methods = append(methods, methodDesc{
				name: method,
				desc: md,
			})
		}
	}
	return methods, nil
}

func (m *Mux) pickMethodConn(name string) (methodConn, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	mcs := m.methods[name]
	if len(mcs) == 0 {
		return methodConn{}, status.Errorf(
			codes.Unimplemented,
			fmt.Sprintf("method %s not implemented", name),
		)
	}
	mc := mcs[rand.Intn(len(mcs))]
	return mc, nil
}

func (m *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	//contentType := r.Header.Get("Content-Type")
	//if r.Method == "POST" && r.ProtoMajor == 2 &&
	//	strings.HasPrefix(contentType, "application/grpc") {
	//	m.serveGRPC(w, r)
	//	return
	//}

	m.serveHTTP(w, r)
}
