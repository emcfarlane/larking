package larking

import (
	"bytes"
	"context"
	"io"
	"math/rand"
	"net/http/httptest"
	"testing"

	"google.golang.org/genproto/googleapis/api/httpbody"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"larking.io/api/testpb"
)

type asHTTPBodyServer struct {
	testpb.UnimplementedFilesServer
}

func (s *asHTTPBodyServer) UploadDownload(ctx context.Context, req *testpb.UploadFileRequest) (*httpbody.HttpBody, error) {
	return &httpbody.HttpBody{
		ContentType: req.File.GetContentType(),
		Data:        req.File.GetData(),
	}, nil
}

// LargeUploadDownload implements testpb.FilesServer
// Echoes the request body as the response body.
func (s *asHTTPBodyServer) LargeUploadDownload(stream testpb.Files_LargeUploadDownloadServer) error {
	var req testpb.UploadFileRequest
	r, err := AsHTTPBodyReader(stream, &req)
	if err != nil {
		return err
	}
	if req.File.Data != nil {
		return status.Error(codes.Internal, "unexpected data")
	}
	if req.File.ContentType != "image/jpeg" {
		return status.Error(codes.Internal, "unexpected content type")
	}
	if req.Filename != "cat.jpg" {
		return status.Error(codes.Internal, "unexpected filename")
	}

	rsp := &httpbody.HttpBody{
		ContentType: req.File.GetContentType(),
	}
	w, err := AsHTTPBodyWriter(stream, rsp)
	if err != nil {
		return err
	}

	n, err := io.Copy(w, r)
	if err != nil {
		return status.Errorf(codes.Internal, "copy error: %d %v", n, err)
	}
	if n == 0 {
		return status.Error(codes.Internal, "zero bytes read")
	}
	return err
}

func TestAsHTTPBody(t *testing.T) {
	// Create test server.
	ts := &asHTTPBodyServer{}

	m, err := NewMux()
	if err != nil {
		t.Fatal(err)
	}
	testpb.RegisterFilesServer(m, ts)

	b := make([]byte, 1024*1024)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	body := bytes.NewReader(b)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/files/large/cat.jpg", body)
	r.Header.Set("Content-Type", "image/jpeg")
	m.ServeHTTP(w, r)

	if w.Code != 200 {
		t.Errorf("unexpected status: %d", w.Code)
		t.Log(w.Body.String())
		return
	}
	if w.Header().Get("Content-Type") != "image/jpeg" {
		t.Errorf("unexpected content type: %s", w.Header().Get("Content-Type"))
	}
	if !bytes.Equal(b, w.Body.Bytes()) {
		t.Errorf("bytes not equal: %d != %d", len(b), len(w.Body.Bytes()))
	}
}
