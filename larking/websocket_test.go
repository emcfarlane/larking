package larking

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/google/go-cmp/cmp"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"larking.io/api/testpb"
)

type wsHeader http.Header

func (h wsHeader) WriteTo(w io.Writer) (int64, error) {
	var buf bytes.Buffer
	if err := http.Header(h).Write(&buf); err != nil {
		return 0, err
	}
	return io.Copy(w, &buf)
}

func TestWebsocket(t *testing.T) {
	// Create test server.
	fs := &testpb.UnimplementedChatRoomServer{}
	o := &overrides{}

	var g errgroup.Group
	defer func() {
		if err := g.Wait(); err != nil {
			t.Fatal(err)
		}
	}()
	mux, err := NewMux(
		UnaryServerInterceptorOption(o.unary()),
		StreamServerInterceptorOption(o.stream()),
	)
	if err != nil {
		t.Fatal(err)
	}
	mux.RegisterService(&testpb.ChatRoom_ServiceDesc, fs)

	s, err := NewServer(mux, InsecureServerOption())
	if err != nil {
		t.Fatal(err)
	}

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()

	g.Go(func() (err error) {
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

	cmpOpts := cmp.Options{protocmp.Transform()}
	var unaryStreamDesc = &grpc.StreamDesc{
		ClientStreams: false,
		ServerStreams: false,
	}

	tests := []struct {
		name   string
		desc   *grpc.StreamDesc
		path   string
		method string
		client []interface{}
		server []interface{}
	}{{
		name:   "unary",
		desc:   unaryStreamDesc,
		path:   "/v1/rooms/chat",
		method: "/larking.testpb.ChatRoom/Chat",
		client: []interface{}{
			in{
				msg: &testpb.ChatMessage{
					Text: "hello",
				},
			},
			out{
				msg: &testpb.ChatMessage{
					Text: "world",
				},
			},
		},
		server: []interface{}{
			in{
				msg: &testpb.ChatMessage{
					Name: "rooms/chat", // name added from URL path
					Text: "hello",
				},
			},
			out{
				msg: &testpb.ChatMessage{
					Text: "world",
				},
			},
		},
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o.reset(t, "http-test", tt.server)

			ctx, cancel := context.WithTimeout(testContext(t), time.Minute)
			defer cancel()

			urlStr := "ws://" + lis.Addr().String() + tt.path

			// TODO: protocols and headers.
			conn, _, _, err := ws.Dialer{
				Header: wsHeader(
					map[string][]string{
						"test": {tt.method},
					},
				),
			}.Dial(ctx, urlStr)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()

			for i := 0; i < len(tt.client); i++ {
				switch typ := tt.client[i].(type) {
				case in:
					b, err := protojson.Marshal(typ.msg)
					if err != nil {
						t.Fatal(err)
					}
					if err := wsutil.WriteClientMessage(conn, ws.OpText, b); err != nil {
						t.Fatal(err)
					}
					t.Log("write", string(b))

				case out:
					b, op, err := wsutil.ReadServerData(conn)
					if err != nil {
						t.Fatal(op, err)
					}
					t.Log("read", string(b))

					out := proto.Clone(typ.msg)
					if err := protojson.Unmarshal(b, out); err != nil {
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
