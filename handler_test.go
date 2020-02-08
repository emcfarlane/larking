package graphpb

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	"github.com/afking/graphpb/grpc/reflection"
	"github.com/afking/graphpb/mock_testpb"
	"github.com/afking/graphpb/testpb"
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

	// Create test server.
	//ts := &testServer{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ms := mock_testpb.NewMockMessagingServer(ctrl)

	gs := grpc.NewServer()
	testpb.RegisterMessagingServer(gs, ms)
	reflection.Register(gs)

	/*h, err := NewHandler(gs)
	if err != nil {
		t.Fatal(err)
	}*/
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	go gs.Serve(lis)
	defer gs.Stop()

	// Create client.
	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	if err != nil {
		t.Fatalf("cannot connect to server: %v", err)
	}
	defer conn.Close()

	h, err := NewMux(conn)
	if err != nil {
		t.Fatal(err)
	}

	// TODO: compare http.Response output
	tests := []struct {
		name       string
		req        *http.Request
		in, out    proto.Message
		method     func(arg0, arg1 interface{}) *gomock.Call
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
		method:     ms.EXPECT().GetMessageOne,
		statusCode: 200,
	}}

	//opts := cmp.Options{protocmp.Transform()}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			/*ts.check = func(ctx context.Context, in interface{}) (interface{}, error) {
				if diff := cmp.Diff(tt.in, in, opts...); diff != "" {
					t.Errorf("mismatch (-want +got):\n%s", diff)
				}
				return tt.out, tt.err
			}*/
			fmt.Println("HERE.........!")
			x := tt.method(gomock.Any(), tt.in).Return(tt.out, tt.err)
			t.Log(x)

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
		})
	}
}
