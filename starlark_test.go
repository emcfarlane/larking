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
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/starlarktest"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

func load(thread *starlark.Thread, module string) (starlark.StringDict, error) {
	if module == "assert.star" {
		return starlarktest.LoadAssertModule()
	}
	return nil, fmt.Errorf("unknown module %s", module)
}

func TestStarlark(t *testing.T) {

	ms := &testpb.UnimplementedMessagingServer{}
	overrides := make(map[string]func(context.Context, proto.Message, string) (proto.Message, error))
	gs := grpc.NewServer(
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
	starlarktest.SetReporter(thread, t)
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
