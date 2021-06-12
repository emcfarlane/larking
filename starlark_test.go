package larking

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"path/filepath"
	"testing"

	"github.com/emcfarlane/larking/grpc/reflection"
	"github.com/emcfarlane/larking/testpb"
	"github.com/emcfarlane/starlarkassert"
	"github.com/google/go-cmp/cmp"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
)

func load(thread *starlark.Thread, module string) (starlark.StringDict, error) {
	if module == "assert.star" {
		return starlarkassert.LoadAssertModule()
	}
	return nil, fmt.Errorf("unknown module %s", module)
}

func TestStarlark(t *testing.T) {
	opts := cmp.Options{protocmp.Transform()}

	ms := &testpb.UnimplementedMessagingServer{}
	gs := grpc.NewServer(
		grpc.UnaryInterceptor(
			func(
				ctx context.Context,
				req interface{},
				info *grpc.UnaryServerInfo,
				handler grpc.UnaryHandler,
			) (interface{}, error) {
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
	reflection.Register(gs)

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()

	var g errgroup.Group
	defer func() {
		if err := g.Wait(); err != nil {
			t.Fatal(err)
		}
	}()

	g.Go(func() error {
		return gs.Serve(lis)
	})
	defer gs.Stop()

	// Create client.
	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	if err != nil {
		t.Fatalf("cannot connect to server: %v", err)
	}
	defer conn.Close()

	mux, err := NewMux()
	if err != nil {
		t.Fatal(err)
	}
	if err := mux.RegisterConn(context.Background(), conn); err != nil {
		t.Fatal(err)
	}

	thread := &starlark.Thread{Load: load}
	starlarkassert.SetReporter(thread, t)
	globals := starlark.StringDict{
		"struct": starlark.NewBuiltin("struct", starlarkstruct.Make),
		//"proto":  starlarkproto.NewModule(),
		"grpc": NewModule(mux),
	}

	files, err := filepath.Glob("testdata/*.star")
	if err != nil {
		t.Fatal(err)
	}

	for _, filename := range files {
		src, err := ioutil.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}

		_, err = starlark.ExecFile(thread, filename, src, globals)
		switch err := err.(type) {
		case *starlark.EvalError:
			var found bool
			for i := range err.CallStack {
				posn := err.CallStack.At(i).Pos
				if posn.Filename() == filename {
					linenum := int(posn.Line)
					msg := err.Error()

					t.Errorf("\n%s:%d: unexpected error: %v", filename, linenum, msg)
					found = true
					break
				}
			}
			if !found {
				t.Error(err.Backtrace())
			}
		case nil:
			// success
		default:
			t.Errorf("\n%s", err)
		}
	}
}
