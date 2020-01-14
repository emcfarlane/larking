package gateway

import (
	"bytes"
	"context"
	"fmt"
	"hash/fnv"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/afking/gateway/google.golang.org/genproto/googleapis/api/annotations"
	rpb "github.com/afking/gateway/grpc/reflection/v1alpha"
)

type Mux struct {
	processed map[protoreflect.FullName]bool
	path      *path
}

//func NewMux(fds ...protoreflect.FileDescriptor) (*Mux, error) {
func NewMux(ccs ...*grpc.ClientConn) (*Mux, error) {
	m := &Mux{
		processed: make(map[protoreflect.FullName]bool),
		path:      newPath(),
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
	stream rpb.ServerReflection_ServerReflectionInfoClient
}

func (r *resolver) FindFileByPath(path string) (protoreflect.FileDescriptor, error) {
	// Standard library? Might not be safe to load locally.
	switch path {
	case annotations.E_Http.Filename:
		return protoregistry.GlobalFiles.FindFileByPath(path)
	}
	// TODO: load remote fds recursively.
	return nil, fmt.Errorf("MISSING %s", path)
}

func (r *resolver) FindDescriptorByName(fullname protoreflect.FullName) (protoreflect.Descriptor, error) {
	return nil, fmt.Errorf("MISSING %s", fullname)
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

	fdbs := make(map[uint32][]byte)
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
			//fmt.Println("GOT FD", string(fdb))
			h := fnv.New32a()
			h.Write(fdb)
			fdbs[h.Sum32()] = fdb
		}
	}

	// Unmarshal file descriptors.
	fds := make(map[string]*descriptorpb.FileDescriptorProto)
	for _, b := range fdbs {
		fd := &descriptorpb.FileDescriptorProto{}

		if err := proto.Unmarshal(b, fd); err != nil {
			return err
		}
		fds[fd.GetName()] = fd
	}

	rslvr := &resolver{stream}

	for _, fd := range fds {
		fd, err := protodesc.NewFile(fd, rslvr)
		if err != nil {
			return err
		}

		if err := m.processFile(cc, fd); err != nil {
			return err
		}
	}

	return nil
}

func (m *Mux) processFile(cc *grpc.ClientConn, fd protoreflect.FileDescriptor) error {
	fmt.Println("processFile", fd.Name())

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
			invoke := func(ctx context.Context, args, reply proto.Message) error {
				return cc.Invoke(ctx, method, args, reply) // TODO: grpc.ClientOpts
			}

			if err := m.path.parseRule(rule, md, invoke); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Println("SERVEHTTP")
	if !strings.HasPrefix(r.URL.Path, "/") {
		r.URL.Path = "/" + r.URL.Path
	}
	//return m, params, nil
	method, params, err := m.path.match(r.URL.Path, r.Method)
	if err != nil {
		http.Error(w, err.Error(), 500) // TODO
		return
	}
	fmt.Println("FOUND", method, params)

	// TODO: fix the body marshalling
	argsDesc := method.desc.Input()
	replyDesc := method.desc.Output()

	args := dynamicpb.NewMessage(argsDesc)
	reply := dynamicpb.NewMessage(replyDesc)
	fmt.Printf("Created %T -> %T\n", args, reply)

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	r.Body.Close()
	fmt.Println("BODY", string(body))

	if len(body) > 0 {
		if err := protojson.Unmarshal(body, args); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}

	// TODO: cleanup paramd decoding
	for _, p := range params {
		cur := args.ProtoReflect()
		for i, fd := range p.fds {
			if len(p.fds)-1 == i {
				cur.Set(fd, p.val)
			} else {
				// TODO: more types
				cur = cur.Mutable(fd).Message()
			}
		}

	}

	if err := method.invoke(r.Context(), args, reply); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	b, err := protojson.Marshal(reply)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err := io.Copy(w, bytes.NewReader(b)); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
}
