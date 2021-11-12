// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package larking

import (
	"context"
	"net"
	"net/http"
	"testing"

	"go.starlark.net/starlark"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"

	"github.com/emcfarlane/larking/api"
	"github.com/emcfarlane/larking/testpb"
	"github.com/google/go-cmp/cmp"
	"golang.org/x/sync/errgroup"
)

func TestServer(t *testing.T) {
	ms := &testpb.UnimplementedMessagingServer{}

	o := &overrides{}
	gs := grpc.NewServer(o.streamOption(), o.unaryOption())
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

	// Create the client.
	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	if err != nil {
		t.Fatalf("cannot connect to server: %v", err)
	}
	defer conn.Close()

	ts, err := NewServer()
	if err != nil {
		t.Fatal(err)
	}
	if err := ts.Mux().RegisterConn(context.Background(), conn); err != nil {
		t.Fatal(err)
	}

	lisProxy, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer lisProxy.Close()

	g.Go(func() error {
		if err := ts.Serve(lisProxy); err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	})
	defer func() {
		if err := ts.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
	}()

	cc, err := grpc.Dial(lisProxy.Addr().String(), grpc.WithInsecure())
	if err != nil {
		t.Fatal(err)
	}

	cmpOpts := cmp.Options{protocmp.Transform()}

	var unaryStreamDesc = &grpc.StreamDesc{
		ClientStreams: false,
		ServerStreams: false,
	}

	tests := []struct {
		name   string
		desc   *grpc.StreamDesc
		method string
		inouts []interface{}
		//ins    []in
		//outs   []out
	}{{
		name:   "unary_message",
		desc:   unaryStreamDesc,
		method: "/larking.testpb.Messaging/GetMessageOne",
		inouts: []interface{}{
			in{msg: &testpb.GetMessageRequestOne{Name: "proxy"}},
			out{msg: &testpb.Message{Text: "success"}},
		},
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o.reset(t, "test", tt.inouts)

			ctx := context.Background()
			ctx = metadata.AppendToOutgoingContext(ctx, "test", tt.method)

			s, err := cc.NewStream(ctx, tt.desc, tt.method)
			if err != nil {
				t.Fatal(err)
			}

			for i := 0; i < len(tt.inouts); i++ {
				switch typ := tt.inouts[i].(type) {
				case in:
					if err := s.SendMsg(typ.msg); err != nil {
						t.Fatal(err)
					}
				case out:
					out := proto.Clone(typ.msg)
					if err := s.RecvMsg(out); err != nil {
						t.Fatal(err)
					}
					diff := cmp.Diff(out, typ.msg, cmpOpts...)
					if diff != "" {
						t.Fatal(diff)
					}
				}
			}
		})
	}
}

func TestAPIServer(t *testing.T) {

	s, err := NewServer()
	if err != nil {
		t.Fatal(err)
	}
	s.threads = map[string]*Thread{
		"default": {
			name:    "default",
			globals: starlark.StringDict{},
		},
	}

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
		if err := s.Serve(lis); err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	})
	defer func() {
		if err := s.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
	}()

	// Create the client.
	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	if err != nil {
		t.Fatalf("cannot connect to server: %v", err)
	}
	defer conn.Close()

	client := api.NewLarkingClient(conn)

	tests := []struct {
		name string
		ins  []*api.Command
		outs []*api.Result
	}{{
		name: "fibonacci",
		ins: []*api.Command{{
			Name: "default",
			Input: `def fibonacci(n):
    res = list(range(n))
    for i in res[2:]:
        res[i] = res[i-2] + res[i-1]
    return res
`,
		}, {
			Input: "fibonacci(10)\n",
		}},
		outs: []*api.Result{{
			Output: "",
		}, {
			Output: "[0, 1, 1, 2, 3, 5, 8, 13, 21, 34]",
		}},
	}}
	ctx := context.Background()
	cmpOpts := cmp.Options{protocmp.Transform()}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			if len(tt.ins) < len(tt.outs) {
				t.Fatal("invalid args")
			}

			stream, err := client.RunOnThread(ctx)
			if err != nil {
				t.Fatal(err)
			}

			for i := 0; i < len(tt.ins); i++ {
				in := tt.ins[i]
				if err := stream.Send(in); err != nil {
					t.Fatal(err)
				}

				out, err := stream.Recv()
				if err != nil {
					t.Fatal(err)
				}
				t.Logf("out: %v", out)

				diff := cmp.Diff(out, tt.outs[i], cmpOpts...)
				if diff != "" {
					t.Error(diff)
				}
			}
		})
	}
	t.Logf("thread: %v", s.threads["default"])
}
