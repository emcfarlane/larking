package larking

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"path/filepath"
	"reflect"
	"testing"

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

func load(thread *starlark.Thread, module string) (starlark.StringDict, error) {
	if module == "assert.star" {
		return starlarkassert.LoadAssertModule(thread)
	}
	return nil, fmt.Errorf("unknown module %s", module)
}

func TestStarlark(t *testing.T) {
	opts := cmp.Options{protocmp.Transform()}

	ms := &testpb.UnimplementedMessagingServer{}
	gs := grpc.NewServer(
		grpc.UnaryInterceptor(
			func(
				_ context.Context,
				req interface{},
				info *grpc.UnaryServerInfo,
				_ grpc.UnaryHandler,
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

type testCallableArgs struct {
	args []string
}

func (c testCallableArgs) String() string        { return "callable_kwargs" }
func (c testCallableArgs) Type() string          { return "callable_kwargs" }
func (c testCallableArgs) Freeze()               {}
func (c testCallableArgs) Truth() starlark.Bool  { return true }
func (c testCallableArgs) Hash() (uint32, error) { return 0, nil }
func (c testCallableArgs) Name() string          { return "callable_kwargs" }
func (c testCallableArgs) CallInternal(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return starlark.None, nil
}
func (c testCallableArgs) ArgNames() []string { return c.args }

func TestAutoComplete(t *testing.T) {
	d := starlark.NewDict(2)
	if err := d.SetKey(starlark.String("key"), starlark.String("value")); err != nil {
		t.Fatal(err)
	}
	mod := &starlarkstruct.Module{
		Name: "hello",
		Members: starlark.StringDict{
			"world": starlark.String("world"),
			"dict":  d,
			"func": testCallableArgs{[]string{
				"kwarg",
			}},
		},
	}

	for _, tt := range []struct {
		name    string
		globals starlark.StringDict
		line    string
		want    []string
	}{{
		name: "simple",
		globals: map[string]starlark.Value{
			"abc": starlark.String("hello"),
		},
		line: "a",
		want: []string{"abc", "abs", "all", "any"},
	}, {
		name: "simple_semi",
		globals: map[string]starlark.Value{
			"abc": starlark.String("hello"),
		},
		line: "abc = \"hello\"; a",
		want: []string{
			"abc = \"hello\"; abc",
			"abc = \"hello\"; abs",
			"abc = \"hello\"; all",
			"abc = \"hello\"; any",
		},
	}, {
		name: "assignment",
		globals: map[string]starlark.Value{
			"abc": starlark.String("hello"),
		},
		line: "abc = a",
		want: []string{
			"abc = abc",
			"abc = abs",
			"abc = all",
			"abc = any",
		},
	}, {
		name: "nest",
		globals: map[string]starlark.Value{
			"hello": mod,
		},
		line: "hello.wo",
		want: []string{"hello.world"},
	}, {
		name: "dict",
		globals: map[string]starlark.Value{
			"abc":   starlark.String("hello"),
			"hello": mod,
		},
		line: "hello.dict[ab",
		want: []string{
			"hello.dict[abc",
			"hello.dict[abs",
		},
	}, {
		name: "dict_string",
		globals: map[string]starlark.Value{
			"hello": mod,
		},
		line: "hello.dict[\"",
		want: []string{"hello.dict[\"key\"]"},
	}, {
		name: "call",
		globals: map[string]starlark.Value{
			"func": testCallableArgs{[]string{
				"arg_one", "arg_two",
			}},
		},
		line: "func(arg_",
		want: []string{"func(arg_one = ", "func(arg_two = "},
	}, {
		name: "call_multi",
		globals: map[string]starlark.Value{
			"func": testCallableArgs{[]string{
				"arg_one", "arg_two",
			}},
		},
		line: "func(arg_one = func(), arg_",
		want: []string{
			"func(arg_one = func(), arg_one = ",
			"func(arg_one = func(), arg_two = ",
		},
	}} {
		t.Run(tt.name, func(t *testing.T) {
			c := Completer{tt.globals}
			got := c.Complete(tt.line)

			if !reflect.DeepEqual(tt.want, got) {
				t.Errorf("%v != %v", tt.want, got)
			}
		})
	}
}
