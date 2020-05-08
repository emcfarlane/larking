package graphpb

import (
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/dynamicpb"
)

// StreamHandler returns a gRPC stream handler to proxy gRPC requests.
func (m *Mux) StreamHandler() grpc.StreamHandler {
	return func(srv interface{}, stream grpc.ServerStream) error {
		// TODO: implement
		name, _ := grpc.MethodFromServerStream(stream)

		mc, err := m.pickMethodConn(name)
		if err != nil {
			return err
		}

		// TODO: non marhsalling codec
		argsDesc := mc.desc.Input()
		replyDesc := mc.desc.Output()

		args := dynamicpb.NewMessage(argsDesc)
		reply := dynamicpb.NewMessage(replyDesc)

		if err := stream.SendMsg(args); err != nil {
			return err
		}

		if err := stream.RecvMsg(reply); err != nil {
			return err
		}

		return nil
	}
}

//func (m *Mux) proxyGRPC(w http.ResponseWriter, r *http.Request) error {
//	if !strings.HasPrefix(r.URL.Path, "/") {
//		r.URL.Path = "/" + r.URL.Path
//	}
//
//	d, err := httputil.DumpRequest(r, true)
//	if err != nil {
//		return err
//	}
//	fmt.Println(string(d))
//
//	name := r.URL.Path
//
//	m.mu.Lock()
//	ccs := m.methods[name]
//	if len(ccs) == 0 {
//		m.mu.Unlock()
//		return fmt.Errorf("missing connections")
//	}
//	cc := ccs[rand.Intn(len(ccs))]
//	m.mu.Unlock()
//
//	md := make(metadata.MD)
//	if grpcTimeout := r.Header.Get("grpc-timeout"); grpcTimeout != "" {
//		md["grpc-timeout"] = []string{grpcTimeout}
//	}
//	// TODO: handle encoding...
//	//if grpcEncoding := r.Header.Get("grpc-encoding"); grpcEncoding
//	if authorization := r.Header.Get("authorization"); authorization != "" {
//		md["authorization"] = []string{authorization}
//	}
//
//	ctx := metadata.NewOutgoingContext(r.Context(), md)
//
//	// Decode body
//
//	if err := cc.Invoke(ctx, name, args, reply); err != nil {
//		return err
//	}
//
//	// Encode body
//
//	return nil
//}
//
//func (m *Mux) serveGRPC(w http.ResponseWriter, r *http.Request) {
//	fl, ok := w.(http.Flusher)
//	if !ok {
//		panic("gRPC requires a ResponseWriter supporting http.Flusher")
//	}
//
//	err := m.proxyGRPC(w, r)
//
//	// Flush headers and body forces seperation of trailers.
//	fl.Flush()
//
//	st, _ := status.FromError(err)
//	h := w.Header()
//	h.Set("Grpc-Status", fmt.Sprintf("%d", st.Code()))
//	if m := st.Message(); m != "" {
//		h.Set("Grpc-Message", url.QueryEscape(m))
//	}
//
//	if p := st.Proto(); p != nil && len(p.Details) > 0 {
//		stBytes, err := proto.Marshal(p)
//		if err != nil {
//			panic(err)
//		}
//
//		stBin := base64.RawStdEncoding.EncodeToString(stBytes)
//		h.Set("Grpc-Status-Details-Bin", stBin)
//	}
//
//	// TODO: custom trailers.
//}
