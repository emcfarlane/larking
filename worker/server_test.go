// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package worker_test

import (
	"context"
	"net"
	"os"
	"testing"

	"github.com/emcfarlane/larking/api/workerpb"
	"github.com/emcfarlane/larking/control"
	"github.com/emcfarlane/larking/worker"

	"github.com/go-logr/logr"
	testing_logr "github.com/go-logr/logr/testing"
	"github.com/google/go-cmp/cmp"
	starlarkmath "go.starlark.net/lib/math"
	"go.starlark.net/starlark"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/testing/protocmp"
)

func testContext(t *testing.T) context.Context {
	ctx := context.Background()
	log := testing_logr.NewTestLogger(t)
	ctx = logr.NewContext(ctx, log)
	return ctx
}

func TestAPIServer(t *testing.T) {
	//log := testing_logr.NewTestLogger(t)

	workerServer := worker.NewServer(
		func(_ *starlark.Thread, module string) (starlark.StringDict, error) {
			if module == "math.star" {
				return starlarkmath.Module.Members, nil
			}
			return nil, os.ErrNotExist
		},
		control.InsecureControlClient{},
	)

	var opts []grpc.ServerOption
	grpcServer := grpc.NewServer(opts...)
	workerpb.RegisterWorkerServer(grpcServer, workerServer)

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
		if err := grpcServer.Serve(lis); err != nil {
			return err
		}
		return nil
	})
	defer grpcServer.GracefulStop()

	// Create the client.
	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	if err != nil {
		t.Fatalf("cannot connect to server: %v", err)
	}
	defer conn.Close()

	client := workerpb.NewWorkerClient(conn)

	tests := []struct {
		name string
		ins  []*workerpb.Command
		outs []*workerpb.Result
	}{{
		name: "fibonacci",
		ins: []*workerpb.Command{{
			Name: "",
			Exec: &workerpb.Command_Input{
				Input: `def fibonacci(n):
	    res = list(range(n))
	    for i in res[2:]:
		res[i] = res[i-2] + res[i-1]
	    return res
`},
		}, {
			Exec: &workerpb.Command_Input{
				Input: "fibonacci(10)\n",
			},
		}},
		outs: []*workerpb.Result{{
			Result: &workerpb.Result_Output{
				Output: &workerpb.Output{
					Output: "",
				},
			},
		}, {
			Result: &workerpb.Result_Output{
				Output: &workerpb.Output{
					Output: "[0, 1, 1, 2, 3, 5, 8, 13, 21, 34]",
				},
			},
		}},
	}, {
		name: "load",
		ins: []*workerpb.Command{{
			Name: "",
			Exec: &workerpb.Command_Input{
				Input: `load("math.star", "pow")`,
			},
		}, {
			Exec: &workerpb.Command_Input{
				Input: "pow(2, 3)",
			},
		}},
		outs: []*workerpb.Result{{
			Result: &workerpb.Result_Output{
				Output: &workerpb.Output{
					Output: "",
				},
			},
		}, {
			Result: &workerpb.Result_Output{
				Output: &workerpb.Output{
					Output: "8.0",
				},
			},
		}},
	}}
	cmpOpts := cmp.Options{protocmp.Transform()}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := testContext(t)

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
	//t.Logf("thread: %v", s.ls.threads["default"])
}
