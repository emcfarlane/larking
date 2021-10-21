package larking

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"net"
	"testing"
	"time"

	"github.com/emcfarlane/larking/testpb"
	"github.com/google/go-cmp/cmp"
	"golang.org/x/sync/errgroup"
	"google.golang.org/genproto/googleapis/api/httpbody"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"nhooyr.io/websocket"
)

func TestWebsocket(t *testing.T) {
	// Create test server.
	fs := &testpb.UnimplementedFilesServer{}
	o := &overrides{}

	var g errgroup.Group
	defer func() {
		if err := g.Wait(); err != nil {
			t.Fatal(err)
		}
	}()

	s, err := NewServer(
		MuxOptions(
			UnaryServerInterceptorOption(o.unary()),
			StreamServerInterceptorOption(o.stream()),
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	testpb.RegisterFilesServer(s, fs)

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()

	g.Go(func() error {
		return s.Serve(lis)
	})
	defer s.Shutdown(context.Background())

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
	}{{
		name:   "unary",
		desc:   unaryStreamDesc,
		method: "/larking.testpb.Files/UploadDownload",
		inouts: []interface{}{
			in{
				msg: &testpb.UploadFileRequest{
					Filename: "cat.jpg",
					File: &httpbody.HttpBody{
						ContentType: "jpg",
						Data:        []byte("large_cat"),
					},
				},
			},
			out{
				msg: &testpb.UploadFileRequest{
					Filename: "cat_small.jpg",
					File: &httpbody.HttpBody{
						ContentType: "jpg",
						Data:        []byte("small_cat"),
					},
				},
			},
		},
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			c, _, err := websocket.Dial(ctx, "ws://"+lis.Addr().String(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close(websocket.StatusInternalError, "the sky is falling")

			for i := 0; i < len(tt.inouts); i++ {
				switch typ := tt.inouts[i].(type) {
				case in:
					w, err := c.Writer(ctx, websocket.MessageText)
					if err != nil {
						t.Fatal(err)
					}
					b, err := protojson.Marshal(typ.msg)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := io.Copy(w, bytes.NewReader(b)); err != nil {
						t.Fatal(err)
					}
					if err := w.Close(); err != nil {
						t.Fatal(err)
					}

				case out:
					_, r, err := c.Reader(ctx)
					if err != nil {
						t.Fatal(err)
					}

					b, err := ioutil.ReadAll(r)
					if err != nil {
						t.Fatal(err)
					}

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
