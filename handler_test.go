// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package larking

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"larking.io/testpb"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestHandler(t *testing.T) {
	ms := &testpb.UnimplementedMessagingServer{}

	req := httptest.NewRequest(http.MethodPatch, "/v1/messages/msg_123", strings.NewReader(
		`{ "text": "Hi!" }`,
	))
	req.Header.Set("Content-Type", "application/json")

	interceptor := func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		_ grpc.UnaryHandler,
	) (interface{}, error) {
		_, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, fmt.Errorf("missing context metadata")
		}

		if info.FullMethod != "/larking.testpb.Messaging/UpdateMessage" {
			return nil, fmt.Errorf("invalid method %s", info.FullMethod)
		}

		in := req.(proto.Message)
		want := &testpb.UpdateMessageRequestOne{
			MessageId: "msg_123",
			Message: &testpb.Message{
				Text: "Hi!",
			},
		}

		if !proto.Equal(want, in) {
			diff := cmp.Diff(in, want, protocmp.Transform())
			t.Fatal(diff)
			return nil, fmt.Errorf("unexpected message")
		}
		return &testpb.Message{Text: "hello, patch!"}, nil
	}

	m, err := NewMux(UnaryServerInterceptorOption(interceptor))
	if err != nil {
		t.Fatal(err)
	}
	testpb.RegisterMessagingServer(m, ms)

	w := httptest.NewRecorder()

	m.ServeHTTP(w, req)
	t.Log(w)
	r := w.Result()
	if r.StatusCode != 200 {
		t.Fatal(r.Status)
	}

	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(data))

	rsp := &testpb.Message{}
	if err := json.Unmarshal(data, rsp); err != nil {
		t.Fatal(err)
	}
	want := &testpb.Message{Text: "hello, patch!"}

	if !proto.Equal(want, rsp) {
		t.Fatal(cmp.Diff(rsp, want, protocmp.Transform()))
	}
}
