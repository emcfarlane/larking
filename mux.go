package graphpb

import (
	"bytes"
	"compress/gzip"
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

	_ "github.com/afking/graphpb/google.golang.org/genproto/googleapis/api/annotations"
	_ "github.com/afking/graphpb/google.golang.org/genproto/googleapis/api/httpbody"
	rpb "github.com/afking/graphpb/grpc/reflection/v1alpha"
	"github.com/afking/graphpb/grpc/status"
)

type Mux struct {
	processed map[protoreflect.FullName]bool
	path      *path
}

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

var isStdFileDescriptor = map[string]bool{
	"google/api/annotations.proto": true,
	"google/api/httpbody.proto":    true,
}

func (r *resolver) FindFileByPath(path string) (protoreflect.FileDescriptor, error) {
	// Standard library? Might not be safe to load locally.
	if isStdFileDescriptor[path] {
		return protoregistry.GlobalFiles.FindFileByPath(path)
	}

	// TODO: load remote fds recursively.
	return nil, fmt.Errorf("MISSING PATH %s", path)
}

func (r *resolver) FindDescriptorByName(fullname protoreflect.FullName) (protoreflect.Descriptor, error) {
	// TODO: is it safe to load all locally?
	desc, err := protoregistry.GlobalFiles.FindDescriptorByName(fullname)
	if err == nil {
		return desc, nil
	}
	return nil, fmt.Errorf("MISSING NAME %s %v", fullname, err)
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

			//fmt.Println("prule", rule)
			if err := m.path.parseRule(rule, md, invoke); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *Mux) serveHTTP(w http.ResponseWriter, r *http.Request) error {
	if !strings.HasPrefix(r.URL.Path, "/") {
		r.URL.Path = "/" + r.URL.Path
	}

	method, params, err := m.path.match(r.URL.Path, r.Method)
	if err != nil {
		return err
	}
	fmt.Println("FOUND", r.URL.Path, method, params)

	// TODO: fix the body marshalling
	argsDesc := method.desc.Input()
	replyDesc := method.desc.Output()
	fmt.Printf("\n%s -> %s\n", argsDesc, replyDesc)

	args := dynamicpb.NewMessage(argsDesc)
	reply := dynamicpb.NewMessage(replyDesc)

	if method.hasBody {
		// TODO: handler should decide what to select on.
		contentType := r.Header.Get("Content-Type")
		contentEncoding := r.Header.Get("Content-Encoding")

		var body io.ReadCloser
		switch contentEncoding {
		case "gzip":
			body, err = gzip.NewReader(r.Body)
			if err != nil {
				return err
			}

		default:
			body = r.Body
		}
		defer body.Close()

		// TODO: mux options.
		b, err := ioutil.ReadAll(io.LimitReader(body, 1024*1024*2))
		if err != nil {
			return err
		}

		//switch contentType {
		//case
		//default: // "application/json"
		//}

		cur := args.ProtoReflect()
		for _, fd := range method.body {
			cur = cur.Mutable(fd).Message()
		}
		fmt.Println("cur:", contentType, cur.Descriptor().FullName())
		fullname := cur.Descriptor().FullName()

		msg := cur.Interface()
		fmt.Printf("body %s %T %v\n", body, msg, method.body)

		switch fullname {
		case "google.api.HttpBody":
			rfl := msg.ProtoReflect()
			fds := rfl.Descriptor().Fields()
			fdContentType := fds.ByName(protoreflect.Name("content_type"))
			fdData := fds.ByName(protoreflect.Name("data"))
			rfl.Set(fdContentType, protoreflect.ValueOfString(contentType))
			rfl.Set(fdData, protoreflect.ValueOfBytes(b))
			// TODO: extensions?

		default:
			// TODO: contentType check?
			if err := protojson.Unmarshal(b, msg); err != nil {
				return err
			}
		}
	}

	queryParams, err := method.parseQueryParams(r.URL.Query())
	if err != nil {
		return err
	}
	params = append(params, queryParams...)

	if err := params.set(args); err != nil {
		return err
	}

	// TODO: header metadata.
	if err := method.invoke(r.Context(), args, reply); err != nil {
		return err
	}

	accept := r.Header.Get("Accept")
	acceptEncoding := r.Header.Get("Accept-Encoding")

	var resp io.Writer
	switch acceptEncoding {
	case "gzip":
		gRsp := gzip.NewWriter(w)
		defer gRsp.Close()
		resp = gRsp

	default:
		resp = w
	}

	cur := reply.ProtoReflect()
	for _, fd := range method.resp {
		cur = cur.Mutable(fd).Message()
	}
	fmt.Println("cur resp:", accept, cur.Descriptor().FullName())
	fullname := cur.Descriptor().FullName()

	msg := cur.Interface()

	switch fullname {
	case "google.api.HttpBody":
		rfl := msg.ProtoReflect()
		fds := rfl.Descriptor().Fields()
		fdContentType := fds.ByName(protoreflect.Name("content_type"))
		fdData := fds.ByName(protoreflect.Name("data"))
		pContentType := rfl.Get(fdContentType)
		pData := rfl.Get(fdData)

		w.Header().Set("Content-Type", pContentType.String())
		if _, err := io.Copy(resp, bytes.NewReader(pData.Bytes())); err != nil {
			return err
		}

	default:
		// TODO: contentType check?
		b, err := protojson.Marshal(msg)
		if err != nil {
			return err
		}

		w.Header().Set("Content-Type", "application/json")
		if _, err := io.Copy(resp, bytes.NewReader(b)); err != nil {
			return err
		}
	}

	if fRsp, ok := w.(http.Flusher); ok {
		fRsp.Flush()
	}
	return nil
}

func (m *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := m.serveHTTP(w, r); err != nil {
		// TODO: check accepts json?

		s, _ := status.FromError(err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(HTTPStatusCode(s.Code()))

		b, err := protojson.Marshal(s.Proto())
		if err != nil {
			panic(err) // ...
		}
		w.Write(b)
	}
}
