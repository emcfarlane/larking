package larking

import (
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/emcfarlane/larking/starlarkthread"
	"github.com/emcfarlane/larking/starlib"
	"github.com/emcfarlane/larking/testpb"
	"github.com/emcfarlane/starlarkassert"
	"github.com/google/go-cmp/cmp"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestStarlark(t *testing.T) {
	opts := cmp.Options{protocmp.Transform()}

	ms := &testpb.UnimplementedMessagingServer{}
	fs := &testpb.UnimplementedFilesServer{}
	gs := grpc.NewServer(
		grpc.UnaryInterceptor(
			func(
				_ context.Context,
				req interface{},
				info *grpc.UnaryServerInfo,
				_ grpc.UnaryHandler,
			) (interface{}, error) {
				// TODO: fix overrides...
				wantMethod := "/larking.testpb.Messaging/GetMessageOne"
				if info.FullMethod != wantMethod {
					return nil, fmt.Errorf("grpc expected %s, got %s", wantMethod, info.FullMethod)
				}

				msg := req.(proto.Message)
				wantIn := &testpb.GetMessageRequestOne{Name: "starlark"}
				diff := cmp.Diff(msg, wantIn, opts...)
				if diff != "" {
					return nil, fmt.Errorf(diff)
				}
				return &testpb.Message{
					MessageId: "starlark",
					Text:      "hello",
					UserId:    "user",
				}, nil
			},
		),
	)
	testpb.RegisterMessagingServer(gs, ms)
	testpb.RegisterFilesServer(gs, fs)
	reflection.Register(gs)

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	t.Cleanup(func() { lis.Close() })

	var g errgroup.Group
	//defer func() {
	t.Cleanup(func() {
		if err := g.Wait(); err != nil {
			t.Fatal(err)
		}
	})

	g.Go(func() error {
		return gs.Serve(lis)
	})
	t.Cleanup(gs.Stop)

	// Create client.
	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	if err != nil {
		t.Fatalf("cannot connect to server: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	//defer conn.Close()

	mux, err := NewMux()
	if err != nil {
		t.Fatal(err)
	}
	if err := mux.RegisterConn(context.Background(), conn); err != nil {
		t.Fatal(err)
	}

	globals := starlark.StringDict{
		"struct": starlark.NewBuiltin("struct", starlarkstruct.Make),
		//"proto":  starlarkproto.NewModule(),
		"mux": mux,
	}
	t.Log("running")
	starlarkassert.RunTests(
		t, "testdata/*.star", globals,
		starlarkthread.AssertOption,
		starlarkassert.WithLoad(starlib.StdLoad),
	)
	defer t.Log("CLOSING")
}
