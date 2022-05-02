// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package larking

import (
	"bytes"
	"context"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/emcfarlane/larking/testpb"
	"golang.org/x/sync/errgroup"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func TestWeb(t *testing.T) {

	// Create test server.
	ms := &testpb.UnimplementedMessagingServer{}

	o := new(overrides)
	gs := grpc.NewServer(o.unaryOption(), o.streamOption())

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

	h, err := NewMux()
	if err != nil {
		t.Fatal(err)
	}
	if err := h.RegisterConn(context.Background(), conn); err != nil {
		t.Fatal(err)
	}

	s, err := NewServer(h, InsecureServerOption())
	if err != nil {
		t.Fatal(err)
	}

	type want struct {
		statusCode int
		body       []byte // either
		// TODO: headers, trailers
	}

	// TODO: compare http.Response output
	tests := []struct {
		name string
		req  *http.Request
		in   in
		out  out
		want want
	}{{
		name: "unary proto request",
		req: func() *http.Request {
			msg := &testpb.GetMessageRequestOne{Name: "name/hello"}
			b, err := proto.Marshal(msg)
			if err != nil {
				t.Fatal(err)
			}

			body := bytes.NewReader(b)
			req := httptest.NewRequest(http.MethodPost, "/larking.testpb.Messaging/GetMessageOne", body)
			req.Header.Set("Content-Type", grpcWeb+"+proto")
			return req
		}(),
		in: in{
			method: "/larking.testpb.Messaging/GetMessageOne",
			msg:    &testpb.GetMessageRequestOne{Name: "name/hello"},
		},
		out: out{
			msg: &testpb.Message{Text: "hello, world!"},
		},
		want: want{
			statusCode: 200,
			body: func() []byte {
				msg := &testpb.Message{Text: "hello, world!"}
				b, err := proto.Marshal(msg)
				if err != nil {
					t.Fatal(err)
				}
				return append(b, 1<<7, 0, 0, 0, 0)
			}(),
		},
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o.reset(t, "http-test", []interface{}{tt.in, tt.out})

			req := tt.req
			req.Header["test"] = []string{tt.in.method}

			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			resp := w.Result()

			b, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}

			if sc := tt.want.statusCode; sc != resp.StatusCode {
				t.Errorf("expected %d got %d", tt.want.statusCode, resp.StatusCode)
				var msg status.Status
				if err := protojson.Unmarshal(b, &msg); err != nil {
					t.Error(err, string(b))
					return
				}
				t.Error("status.code", msg.Code)
				t.Error("status.message", msg.Message)
				return
			}

			if tt.want.body != nil {
				if !bytes.Equal(b, tt.want.body) {
					t.Errorf("length %d != %d", len(tt.want.body), len(b))
					t.Errorf("body %s != %s", tt.want.body, b)
				}
			}
		})
	}
}
