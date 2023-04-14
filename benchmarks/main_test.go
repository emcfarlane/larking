package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/soheilhy/cmux"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"larking.io/benchmarks/api/librarypb"
	"larking.io/larking"
)

type testService struct {
	librarypb.UnimplementedLibraryServiceServer
}

func (testService) GetBook(ctx context.Context, req *librarypb.GetBookRequest) (*librarypb.Book, error) {
	if req.Name != "shelves/1/books/1" {
		return nil, grpc.Errorf(codes.NotFound, "not found")
	}
	return &librarypb.Book{
		Name:        "shelves/1/books/1",
		Title:       "The Great Gatsby",
		Author:      "F. Scott Fitzgerald",
		PageCount:   180,
		PublishTime: timestamppb.New(time.Now()),
		Duration:    durationpb.New(1 * time.Hour),
		Price:       wrapperspb.Double(9.99),
	}, nil
}

func (testService) ListBooks(ctx context.Context, req *librarypb.ListBooksRequest) (*librarypb.ListBooksResponse, error) {
	if req.Parent != "shelves/1" {
		return nil, grpc.Errorf(codes.NotFound, "not found")
	}
	return &librarypb.ListBooksResponse{
		Books: []*librarypb.Book{
			{
				Name:  "shelves/1/books/1",
				Title: "The Great Gatsby",
			},
			{
				Name:  "shelves/1/books/2",
				Title: "The Catcher in the Rye",
			},
			{
				Name:  "shelves/1/books/3",
				Title: "The Grapes of Wrath",
			},
		},
	}, nil
}

func (testService) CreateBook(ctx context.Context, req *librarypb.CreateBookRequest) (*librarypb.Book, error) {
	return req.Book, nil
}
func (testService) UpdateBook(ctx context.Context, req *librarypb.UpdateBookRequest) (*librarypb.Book, error) {
	if req.Book.GetName() != "shelves/1/books/1" {
		return nil, grpc.Errorf(codes.NotFound, "not found")
	}
	if req.UpdateMask.Paths[0] != "book.title" {
		return nil, grpc.Errorf(codes.InvalidArgument, "invalid field mask")
	}
	return req.Book, nil
}
func (testService) DeleteBook(ctx context.Context, req *librarypb.DeleteBookRequest) (*emptypb.Empty, error) {
	if req.Name != "shelves/1/books/1" {
		return nil, grpc.Errorf(codes.NotFound, "not found")
	}
	return &emptypb.Empty{}, nil
}

func benchGRPC_GetBook(b *testing.B, client librarypb.LibraryServiceClient) {
	ctx := context.Background()
	var rsp *librarypb.Book
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m, err := client.GetBook(ctx, &librarypb.GetBookRequest{Name: "shelves/1/books/1"})
		if err != nil {
			b.Fatal(err)
		}
		rsp = m
	}
	b.StopTimer()
	_ = rsp
}

func doRequest(b *testing.B, method, url string, in, out proto.Message) {
	b.Helper()

	var body io.Reader
	if in != nil {
		p, err := protojson.Marshal(in)
		if err != nil {
			b.Fatal(err)
		}
		body = bytes.NewReader(p)
	}
	r, err := http.NewRequest(method, url, body)
	if err != nil {
		b.Fatal(err)
	}

	w, err := http.DefaultClient.Do(r)
	if err != nil {
		b.Fatal(err)
	}

	buf, err := io.ReadAll(w.Body)
	if err := w.Body.Close(); err != nil {
		b.Fatal(err)
	}
	if err != nil {
		b.Fatal(err)
	}
	if w.StatusCode != http.StatusOK {
		b.Logf("body: %s", body)
		b.Fatalf("status code: %d", w.StatusCode)
	}
	if err := protojson.Unmarshal(buf, out); err != nil {
		b.Fatal(err)
	}
}

func benchHTTP_GetBook(b *testing.B, addr string) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out := &librarypb.Book{}
		doRequest(b, http.MethodGet, "http://"+addr+"/v1/shelves/1/books/1", nil, out)
	}
	b.StopTimer()
}

func benchHTTP_UpdateBook(b *testing.B, addr string) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		in := &librarypb.Book{
			Name:  "shelves/1/books/1",
			Title: "The Great Gatsby",
		}
		out := &librarypb.Book{}

		doRequest(b, http.MethodPatch, "http://"+addr+"/v1/shelves/1/books/1?update_mask=book.title", in, out)
	}
	b.StopTimer()
}

func benchHTTP_DeleteBook(b *testing.B, addr string) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out := &emptypb.Empty{}

		doRequest(b, http.MethodDelete, "http://"+addr+"/v1/shelves/1/books/1", nil, out)
	}
	b.StopTimer()
}

func BenchmarkLarking(b *testing.B) {
	ctx := context.Background()
	svc := &testService{}

	mux, err := larking.NewMux()
	if err != nil {
		b.Fatal(err)
	}
	librarypb.RegisterLibraryServiceServer(mux, svc)

	ts, err := larking.NewServer(mux, larking.InsecureServerOption())
	if err != nil {
		b.Fatal(err)
	}

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		b.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()

	var g errgroup.Group
	defer func() {
		if err := g.Wait(); err != nil {
			b.Fatal(err)
		}
		b.Log("all server shutdown")
	}()

	g.Go(func() (err error) {
		if err := ts.Serve(lis); err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	})
	defer func() {
		b.Log("shutdown server")
		if err := ts.Shutdown(ctx); err != nil {
			b.Fatal(err)
		}
	}()

	cc, err := grpc.Dial(
		lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(time.Second),
	)
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		if err := cc.Close(); err != nil {
			b.Fatal(err)
		}
	}()
	client := librarypb.NewLibraryServiceClient(cc)

	b.Run("GRPC_GetBook", func(b *testing.B) {
		benchGRPC_GetBook(b, client)
	})
	b.Run("HTTP_GetBook", func(b *testing.B) {
		benchHTTP_GetBook(b, lis.Addr().String())
	})
	b.Run("HTTP_UpdateBook", func(b *testing.B) {
		benchHTTP_UpdateBook(b, lis.Addr().String())
	})
	b.Run("HTTP_DeleteBook", func(b *testing.B) {
		benchHTTP_DeleteBook(b, lis.Addr().String())
	})
}

func BenchmarkGRPCGateway(b *testing.B) {
	ctx := context.Background()
	svc := &testService{}

	gs := grpc.NewServer(grpc.Creds(insecure.NewCredentials()))
	librarypb.RegisterLibraryServiceServer(gs, svc)

	mux := runtime.NewServeMux()
	if err := librarypb.RegisterLibraryServiceHandlerServer(ctx, mux, svc); err != nil {
		b.Fatal(err)
	}

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		b.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()

	// Create the cmux object that will multiplex 2 protocols on the same port.
	// The two following listeners will be served on the same port below gracefully.
	m := cmux.New(lis)
	// Match gRPC requests here
	grpcL := m.MatchWithWriters(
		cmux.HTTP2MatchHeaderFieldSendSettings("content-type", "application/grpc"),
	)
	// Otherwise match regular http requests.
	httpL := m.Match(cmux.Any())

	n := 3
	errs := make(chan error, n)

	go func() { errs <- gs.Serve(grpcL) }()
	defer gs.Stop()

	hs := &http.Server{
		Handler: mux,
	}
	go func() { errs <- hs.Serve(httpL) }()
	defer hs.Close()

	go func() { errs <- m.Serve() }()

	cc, err := grpc.Dial(
		lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(time.Second),
	)
	if err != nil {
		b.Fatal(err)
	}
	defer cc.Close()
	client := librarypb.NewLibraryServiceClient(cc)

	b.Run("GRPC_GetBook", func(b *testing.B) {
		benchGRPC_GetBook(b, client)
	})
	b.Run("HTTP_GetBook", func(b *testing.B) {
		benchHTTP_GetBook(b, lis.Addr().String())
	})
	b.Run("HTTP_UpdateBook", func(b *testing.B) {
		benchHTTP_UpdateBook(b, lis.Addr().String())
	})
	b.Run("HTTP_DeleteBook", func(b *testing.B) {
		benchHTTP_DeleteBook(b, lis.Addr().String())
	})
}
