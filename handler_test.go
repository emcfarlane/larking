package gateway

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/afking/gateway/testpb"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc"
)

type testServer struct {
	check func(context.Context, interface{}) (interface{}, error)
	//msg *testpb.Message
	//err error
}

func (ts *testServer) GetMessageOne(ctx context.Context, req *testpb.GetMessageRequestOne) (*testpb.Message, error) {
	v, err := ts.check(ctx, req)
	return v.(*testpb.Message), err
}

func (ts *testServer) GetMessageTwo(ctx context.Context, req *testpb.GetMessageRequestTwo) (*testpb.Message, error) {
	v, err := ts.check(ctx, req)
	return v.(*testpb.Message), err
}

func (ts *testServer) UpdateMessage(ctx context.Context, req *testpb.UpdateMessageRequestOne) (*testpb.Message, error) {
	v, err := ts.check(ctx, req)
	return v.(*testpb.Message), err
}

func (ts *testServer) UpdateMessageBody(ctx context.Context, req *testpb.Message) (*testpb.Message, error) {
	v, err := ts.check(ctx, req)
	return v.(*testpb.Message), err
}

func TestMessageServer(t *testing.T) {
	ts := &testServer{}

	gs := grpc.NewServer()
	testpb.RegisterMessagingServer(gs, ts)

	h, err := NewHandler(gs)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		req        *http.Request
		in, out    interface{}
		err        error
		statusCode int
	}{{
		name: "first",
		req:  httptest.NewRequest(http.MethodGet, "/v1/messages/name/hello", nil),
		in: &testpb.GetMessageRequestOne{
			Name: "name/hello",
		},
		out:        &testpb.Message{Text: "hello, world!"},
		err:        nil,
		statusCode: 200,
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts.check = func(ctx context.Context, in interface{}) (interface{}, error) {
				if diff := cmp.Diff(tt.in, in); diff != "" {
					t.Errorf("mismatch (-want +got):\n%s", diff)
				}
				return tt.out, tt.err
			}

			w := httptest.NewRecorder()
			h.ServeHTTP(w, tt.req)
			resp := w.Result()

			b, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			t.Log("body:", string(b))
			t.Logf("resp: %+v\n", resp)

			if tt.statusCode != resp.StatusCode {
				t.Errorf("expected %d got %d", tt.statusCode, resp.StatusCode)
			}

			t.Fail()
		})
	}
}
