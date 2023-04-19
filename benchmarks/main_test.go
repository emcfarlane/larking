package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/soheilhy/cmux"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"larking.io/benchmarks/api/librarypb"
	"larking.io/larking"
)

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

func BenchmarkEnvoyGRPC(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())

	if err := exec.CommandContext(ctx, "which", "envoy").Run(); err != nil {
		b.Log(err)
		b.Skip("envoy is not ready")
	}

	svc := &testService{}

	gs := grpc.NewServer(grpc.Creds(insecure.NewCredentials()))
	librarypb.RegisterLibraryServiceServer(gs, svc)

	envoyAddr := "localhost:10000"

	lis, err := net.Listen("tcp", "localhost:5050")
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
		if err := gs.Serve(lis); err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	})
	defer gs.Stop()

	g.Go(func() (err error) {
		cmd := exec.CommandContext(ctx, "envoy", "-c", "testdata/envoy.yaml")

		var out strings.Builder
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			b.Log(err)
		}
		//b.Log(out.String())
		return nil
	})
	defer cancel()

	cc, err := grpc.Dial(
		envoyAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(2*time.Second),
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
		benchHTTP_GetBook(b, envoyAddr)
	})
	b.Run("HTTP_UpdateBook", func(b *testing.B) {
		benchHTTP_UpdateBook(b, envoyAddr)
	})
	b.Run("HTTP_DeleteBook", func(b *testing.B) {
		benchHTTP_DeleteBook(b, envoyAddr)
	})
}
