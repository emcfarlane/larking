package graphpb

import (
	"context"
	"net"
	"testing"

	//"google.golang.org/genproto/googleapis/api/httpbody" // TODO
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	"github.com/afking/graphpb/grpc/reflection"
	"github.com/afking/graphpb/testpb"
)

func TestGRPCProxy(t *testing.T) {
	// Create test server.
	ms := &testpb.UnimplementedMessagingServer{}

	overrides := make(map[string]func(context.Context, proto.Message, string) (proto.Message, error))
	gs := grpc.NewServer(
		grpc.StreamInterceptor(
			func(
				srv interface{},
				stream grpc.ServerStream,
				info *grpc.StreamServerInfo,
				handler grpc.StreamHandler,
			) (err error) {
				return handler(srv, stream)
			},
		),
		grpc.UnaryInterceptor(
			func(
				ctx context.Context,
				req interface{},
				info *grpc.UnaryServerInfo,
				handler grpc.UnaryHandler,
			) (interface{}, error) {
				md, ok := metadata.FromIncomingContext(ctx)
				if !ok {
					return handler(ctx, req) // default
				}
				ss := md["test"]
				if len(ss) == 0 {
					return handler(ctx, req) // default
				}
				h, ok := overrides[ss[0]]
				if !ok {
					return handler(ctx, req) // default
				}

				// TODO: reflection assert on handler types.
				return h(ctx, req.(proto.Message), info.FullMethod)
			},
		),
	)
	testpb.RegisterMessagingServer(gs, ms)
	reflection.Register(gs)

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()

	go gs.Serve(lis)
	defer gs.Stop()

	// Create the client.
	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	if err != nil {
		t.Fatalf("cannot connect to server: %v", err)
	}
	defer conn.Close()

	h, err := NewMux(conn)
	if err != nil {
		t.Fatal(err)
	}

	lisProxy, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer lisProxy.Close()

	ts := grpc.NewServer(
		grpc.UnknownServiceHandler(h.StreamHandler()),
	)

	go ts.Serve(lisProxy)
	defer ts.Stop()

	//ts := httptest.NewUnstartedServer(h)
	//ts.EnableHTTP2 = true
	//ts.StartTLS()
	//defer ts.Close()

	//transport := ts.Client().Transport.(*http.Transport)

	cc, err := grpc.Dial(
		lisProxy.Addr().String(),
		//grpc.WithTransportCredentials(
		//	credentials.NewTLS(transport.TLSClientConfig),
		//),
		grpc.WithInsecure(),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(cc.GetState())

	client := testpb.NewMessagingClient(cc)

	ctx := context.Background()
	in := &testpb.GetMessageRequestOne{}
	out, err := client.GetMessageOne(ctx, in, grpc.WaitForReady(true))
	if err != nil {
		t.Log("here")
		t.Fatal(err)
	}
	t.Log(out)
}
