package graphpb

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/emcfarlane/graphpb/testpb"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestHandler(t *testing.T) {
	ms := &testpb.UnimplementedMessagingServer{}

	var h Handler
	if err := h.RegisterServiceByName("graphpb.testpb.Messaging", ms); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPatch, "/v1/messages/msg_123", strings.NewReader(
		`{ "text": "Hi!" }`,
	))
	req.Header.Set("Content-Type", "application/json")

	h.UnaryInterceptor = func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		_, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, fmt.Errorf("missing context metadata")
		}

		if info.FullMethod != "/graphpb.testpb.Messaging/UpdateMessage" {
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

	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
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
