// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package larking

import (
	"bytes"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"larking.io/api/testpb"
)

func TestWeb(t *testing.T) {

	// Create test server.
	ms := &testpb.UnimplementedMessagingServer{}

	o := new(overrides)
	gs := grpc.NewServer(o.unaryOption(), o.streamOption())

	testpb.RegisterMessagingServer(gs, ms)
	h := createGRPCWebHandler(gs)

	type want struct {
		statusCode int
		//body       []byte // either
		msg proto.Message // or
		// TODO: headers, trailers
	}

	frame := func(b []byte, msb uint8) []byte {
		head := append([]byte{0 | msb, 0, 0, 0, 0}, b...)
		binary.BigEndian.PutUint32(head[1:5], uint32(len(b)))
		return head
	}
	deframe := func(b []byte) ([]byte, []byte) {
		if len(b) < 5 {
			t.Errorf("invalid deframe")
			return nil, nil
		}
		x := int(binary.BigEndian.Uint32(b[1:5]))
		b = b[5:]
		return b[:x], b[x:]
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

			body := bytes.NewReader(frame(b, 0))
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
			msg:        &testpb.Message{Text: "hello, world!"},
		},
	}}

	opts := cmp.Options{protocmp.Transform()}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o.reset(t, "test", []interface{}{tt.in, tt.out})

			req := tt.req
			req.Header["test"] = []string{tt.in.method}

			w := httptest.NewRecorder()
			//s.gs.ServeHTTP(w, req)
			h.ServeHTTP(w, req)
			resp := w.Result()

			t.Log("resp", resp)

			b, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("resp length: %d", len(b))

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

			//if tt.want.body != nil {
			//	if !bytes.Equal(b, tt.want.body) {
			//		t.Errorf("length %d != %d", len(tt.want.body), len(b))
			//		t.Errorf("body %s != %s", tt.want.body, b)
			//	}
			//}
			if tt.want.msg != nil {
				b, _ := deframe(b)
				msg := proto.Clone(tt.want.msg)
				if err := proto.Unmarshal(b, msg); err != nil {
					t.Errorf("%v: %X", err, b)
					return
				}
				diff := cmp.Diff(msg, tt.want.msg, opts...)
				if diff != "" {
					t.Error(diff)
				}
			}
		})
	}
}
