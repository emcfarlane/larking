package larking

import (
	"context"
	"fmt"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"

	"github.com/emcfarlane/larking/grpc/reflection"
	"github.com/emcfarlane/larking/testpb"
	"github.com/google/go-cmp/cmp"
	"golang.org/x/sync/errgroup"
)

func TestServer(t *testing.T) {
	ms := &testpb.UnimplementedMessagingServer{}

	overrides := make(overrides)
	gs := grpc.NewServer(
		overrides.StreamInterceptor(),
		overrides.UnaryInterceptor(),
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
		return ts.Serve(lisProxy)
	})
	//defer ts.Stop()
	defer ts.Close()

	cc, err := grpc.Dial(lisProxy.Addr().String(), grpc.WithInsecure())
	if err != nil {
		t.Fatal(err)
	}

	type in struct {
		msg proto.Message
	}

	type out struct {
		msg proto.Message
		err error
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
		ins    []in
		outs   []out
	}{{
		name:   "unary_message",
		desc:   unaryStreamDesc,
		method: "/larking.testpb.Messaging/GetMessageOne",
		ins: []in{{
			msg: &testpb.GetMessageRequestOne{
				Name: "proxy",
			},
		}},
		outs: []out{{
			msg: &testpb.Message{Text: "success"},
		}},
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				i int
				o override
			)

			o.unary = func(
				ctx context.Context,
				msg proto.Message,
				method string,
			) (proto.Message, error) {
				if method != tt.method {
					return nil, fmt.Errorf("grpc expected %s, got %s", tt.method, method)
				}

				diff := cmp.Diff(msg, tt.ins[i].msg, cmpOpts...)
				if diff != "" {
					return nil, fmt.Errorf(diff)
				}
				return tt.outs[i].msg, tt.outs[i].err
			}
			overrides[t.Name()] = o
			defer delete(overrides, t.Name())

			ctx := context.Background()
			ctx = metadata.AppendToOutgoingContext(ctx, "test", t.Name())

			s, err := cc.NewStream(ctx, tt.desc, tt.method)
			if err != nil {
				t.Fatal(err)
			}

			for ; i < len(tt.ins); i++ {
				if err := s.SendMsg(tt.ins[i].msg); err != nil {
					t.Fatal(err)
				}

				out := proto.Clone(tt.outs[i].msg)
				if err := s.RecvMsg(out); err != nil {
					t.Fatal(err)
				}

				diff := cmp.Diff(out, tt.outs[i].msg, cmpOpts...)
				if diff != "" {
					t.Fatal(diff)
				}
			}
		})
	}
}
