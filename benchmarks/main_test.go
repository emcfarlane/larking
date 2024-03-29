package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	connect_go "github.com/bufbuild/connect-go"
	"github.com/gorilla/mux"
	gwruntime "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/soheilhy/cmux"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"golang.org/x/sync/errgroup"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"larking.io/benchmarks/api/librarypb"
	cpb "larking.io/benchmarks/api/librarypb/librarypbconnect"
	"larking.io/larking"
)

const procs = 8

var client = &http.Client{
	Transport: &http.Transport{
		// just what default client uses
		Proxy: http.ProxyFromEnvironment,
		// this leads to more stable numbers
		MaxIdleConnsPerHost: procs * runtime.GOMAXPROCS(0),
	},
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

var (
	jsonCodec  = larking.CodecJSON{}
	protoCodec = larking.CodecProto{}
)

func doRequest(b *testing.B, method, url string, in, out proto.Message, isProto bool) {
	b.Helper()

	var codec larking.Codec = &jsonCodec
	if isProto {
		codec = &protoCodec
	}

	var body io.Reader
	if in != nil {
		p, err := codec.Marshal(in)
		if err != nil {
			b.Fatal(err)
		}
		body = bytes.NewReader(p)
	}
	r, err := http.NewRequest(method, url, body)
	if err != nil {
		b.Fatal(err)
	}
	contentType := "application/json"
	if isProto {
		contentType = "application/protobuf"
	}
	r.Header.Set("Content-Type", contentType)
	r.Header.Set("Accept-Encoding", "identity")

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
		b.Log(r.URL.String())
		b.Logf("body: %s", buf)
		b.Fatalf("status code: %d", w.StatusCode)
	}
	if err := codec.Unmarshal(buf, out); err != nil {
		b.Fatal(err)
	}
}

func benchHTTP_GetBook(b *testing.B, addr string, isProto bool) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out := &librarypb.Book{}
		doRequest(b, http.MethodGet, "http://"+addr+"/v1/shelves/1/books/1", nil, out, isProto)
	}
	b.StopTimer()
}

func benchHTTP_UpdateBook(b *testing.B, addr string, isProto bool) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		in := &librarypb.Book{
			Name:  "shelves/1/books/1",
			Title: "The Great Gatsby",
		}
		out := &librarypb.Book{}

		doRequest(b, http.MethodPatch, "http://"+addr+"/v1/shelves/1/books/1?update_mask=book.title", in, out, isProto)
	}
	b.StopTimer()
}

func benchHTTP_DeleteBook(b *testing.B, addr string, isProto bool) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out := &emptypb.Empty{}

		doRequest(b, http.MethodDelete, "http://"+addr+"/v1/shelves/1/books/1", nil, out, isProto)
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

	ts, err := larking.NewServer(mux)
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
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cc, err := grpc.DialContext(
		ctx,
		lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
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
		benchHTTP_GetBook(b, lis.Addr().String(), false)
	})
	b.Run("HTTP_UpdateBook", func(b *testing.B) {
		benchHTTP_UpdateBook(b, lis.Addr().String(), false)
	})
	b.Run("HTTP_DeleteBook", func(b *testing.B) {
		benchHTTP_DeleteBook(b, lis.Addr().String(), false)
	})
	b.Run("HTTP_GetBook+pb", func(b *testing.B) {
		benchHTTP_GetBook(b, lis.Addr().String(), true)
	})
	b.Run("HTTP_UpdateBook+pb", func(b *testing.B) {
		benchHTTP_UpdateBook(b, lis.Addr().String(), true)
	})
	b.Run("HTTP_DeleteBook+pb", func(b *testing.B) {
		benchHTTP_DeleteBook(b, lis.Addr().String(), true)
	})
}

func BenchmarkGRPCGateway(b *testing.B) {
	ctx := context.Background()
	svc := &testService{}

	gs := grpc.NewServer(grpc.Creds(insecure.NewCredentials()))
	librarypb.RegisterLibraryServiceServer(gs, svc)

	mux := gwruntime.NewServeMux()
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

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cc, err := grpc.DialContext(
		ctx,
		lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
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
		benchHTTP_GetBook(b, lis.Addr().String(), false)
	})
	b.Run("HTTP_UpdateBook", func(b *testing.B) {
		benchHTTP_UpdateBook(b, lis.Addr().String(), false)
	})
	b.Run("HTTP_DeleteBook", func(b *testing.B) {
		benchHTTP_DeleteBook(b, lis.Addr().String(), false)
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

	cc, err := grpc.DialContext(
		ctx,
		envoyAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
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
		benchHTTP_GetBook(b, envoyAddr, false)
	})
	b.Run("HTTP_UpdateBook", func(b *testing.B) {
		benchHTTP_UpdateBook(b, envoyAddr, false)
	})
	b.Run("HTTP_DeleteBook", func(b *testing.B) {
		benchHTTP_DeleteBook(b, envoyAddr, false)
	})
}

func BenchmarkGorillaMux(b *testing.B) {
	ctx := context.Background()
	svc := &testService{}

	writeErr := func(w http.ResponseWriter, err error) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error())) //nolint
	}

	newIncomingContext := func(ctx context.Context, header http.Header) (context.Context, metadata.MD) {
		md := make(metadata.MD, len(header))
		for k, vs := range header {
			md[strings.ToLower(k)] = vs
		}
		return metadata.NewIncomingContext(ctx, md), md
	}

	r := mux.NewRouter()
	r.HandleFunc("/v1/shelves/{shelfID}/books/{bookID}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		shelfID := vars["shelfID"]
		bookID := vars["bookID"]
		name := fmt.Sprintf("shelves/%s/books/%s", shelfID, bookID)

		ctx := r.Context()
		ctx, _ = newIncomingContext(ctx, r.Header)

		msg, err := svc.GetBook(ctx, &librarypb.GetBookRequest{Name: name})
		if err != nil {
			writeErr(w, err)
			return
		}
		b, err := protojson.Marshal(msg)
		if err != nil {
			writeErr(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(b) //nolint
	}).Methods(http.MethodGet)

	r.HandleFunc("/v1/shelves/{shelfID}/books/{bookID}", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		vars := mux.Vars(r)
		shelfID := vars["shelfID"]
		bookID := vars["bookID"]
		name := fmt.Sprintf("shelves/%s/books/%s", shelfID, bookID)

		ctx := r.Context()
		ctx, _ = newIncomingContext(ctx, r.Header)

		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeErr(w, err)
			return
		}

		var book librarypb.Book
		if err := protojson.Unmarshal(body, &book); err != nil {
			writeErr(w, err)
			return
		}
		book.Name = name

		var updateMask fieldmaskpb.FieldMask
		updateMaskRaw := r.URL.Query().Get("update_mask")

		if updateMaskRaw != "" {
			mv := []byte(strconv.Quote(updateMaskRaw))
			if err := protojson.Unmarshal(mv, &updateMask); err != nil {
				writeErr(w, err)
				return
			}
		}

		msg, err := svc.UpdateBook(ctx, &librarypb.UpdateBookRequest{
			Book:       &book,
			UpdateMask: &updateMask,
		})
		if err != nil {
			writeErr(w, err)
			return
		}
		b, err := protojson.Marshal(msg)
		if err != nil {
			writeErr(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(b) //nolint
	}).Methods(http.MethodPatch)

	r.HandleFunc("/v1/shelves/{shelfID}/books/{bookID}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		shelfID := vars["shelfID"]
		bookID := vars["bookID"]
		name := fmt.Sprintf("shelves/%s/books/%s", shelfID, bookID)

		ctx := r.Context()
		ctx, _ = newIncomingContext(ctx, r.Header)

		msg, err := svc.DeleteBook(ctx, &librarypb.DeleteBookRequest{Name: name})
		if err != nil {
			writeErr(w, err)
			return
		}
		b, err := protojson.Marshal(msg)
		if err != nil {
			writeErr(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(b) //nolint
	}).Methods(http.MethodDelete)

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

	s := &http.Server{
		Handler: r,
	}

	g.Go(func() (err error) {
		if err := s.Serve(lis); err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	})
	defer func() {
		b.Log("shutdown server")
		if err := s.Shutdown(ctx); err != nil {
			b.Fatal(err)
		}
	}()

	b.Run("HTTP_GetBook", func(b *testing.B) {
		benchHTTP_GetBook(b, lis.Addr().String(), false)
	})
	b.Run("HTTP_UpdateBook", func(b *testing.B) {
		benchHTTP_UpdateBook(b, lis.Addr().String(), false)
	})
	b.Run("HTTP_DeleteBook", func(b *testing.B) {
		benchHTTP_DeleteBook(b, lis.Addr().String(), false)
	})
}

func BenchmarkConnectGo(b *testing.B) {
	ctx := context.Background()
	svc := &testConnectService{}

	mux := http.NewServeMux()
	path, handler := cpb.NewLibraryServiceHandler(svc)
	mux.Handle(path, handler)

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		b.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()

	n := 1
	errs := make(chan error, n)

	h2s := &http2.Server{}
	hs := &http.Server{
		Handler: h2c.NewHandler(mux, h2s),
	}

	go func() { errs <- hs.Serve(lis) }()
	defer hs.Close()

	cc, err := grpc.DialContext(
		ctx,
		lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		b.Fatal(err)
	}
	defer cc.Close()
	client := librarypb.NewLibraryServiceClient(cc)

	ccc := cpb.NewLibraryServiceClient(
		http.DefaultClient,
		"http://"+lis.Addr().String(),
	)

	b.Run("GRPC_GetBook", func(b *testing.B) {
		benchGRPC_GetBook(b, client)
	})
	b.Run("HTTP_GetBook", func(b *testing.B) {
		addr := lis.Addr().String()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			in := &librarypb.GetBookRequest{
				Name: "shelves/1/books/1",
			}
			out := &librarypb.Book{}
			doRequest(b, http.MethodPost, "http://"+addr+cpb.LibraryServiceGetBookProcedure, in, out, false)
		}
		b.StopTimer()
	})
	b.Run("HTTP_UpdateBook", func(b *testing.B) {
		addr := lis.Addr().String()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			in := &librarypb.UpdateBookRequest{
				Book: &librarypb.Book{
					Name:  "shelves/1/books/1",
					Title: "The Great Gatsby",
				},
				UpdateMask: &field_mask.FieldMask{
					Paths: []string{"book.title"},
				},
			}
			out := &librarypb.Book{}
			doRequest(b, http.MethodPost, "http://"+addr+cpb.LibraryServiceUpdateBookProcedure, in, out, false)
		}
		b.StopTimer()
	})
	b.Run("HTTP_DeleteBook", func(b *testing.B) {
		addr := lis.Addr().String()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			in := &librarypb.DeleteBookRequest{
				Name: "shelves/1/books/1",
			}
			out := &emptypb.Empty{}
			doRequest(b, http.MethodPost, "http://"+addr+cpb.LibraryServiceDeleteBookProcedure, in, out, false)
		}
		b.StopTimer()
	})
	b.Run("Connect_GetBook", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			in := &librarypb.GetBookRequest{
				Name: "shelves/1/books/1",
			}
			_, err := ccc.GetBook(ctx, connect_go.NewRequest(in))
			if err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
	})
	b.Run("Connect_UpdateBook", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			in := &librarypb.UpdateBookRequest{
				Book: &librarypb.Book{
					Name:  "shelves/1/books/1",
					Title: "The Great Gatsby",
				},
				UpdateMask: &field_mask.FieldMask{
					Paths: []string{"book.title"},
				},
			}
			_, err := ccc.UpdateBook(ctx, connect_go.NewRequest(in))
			if err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
	})
	b.Run("Connect_DeleteBook", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			in := &librarypb.DeleteBookRequest{
				Name: "shelves/1/books/1",
			}
			_, err := ccc.DeleteBook(ctx, connect_go.NewRequest(in))
			if err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
	})
}

func BenchmarkTwirp(b *testing.B) {
	ctx := context.Background()
	svc := &testService{}

	svr := librarypb.NewLibraryServiceServer(svc)

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		b.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()

	n := 1
	errs := make(chan error, n)

	hs := &http.Server{
		Handler: svr,
	}

	go func() { errs <- hs.Serve(lis) }()
	defer hs.Close()

	ccc := librarypb.NewLibraryServiceJSONClient(
		"http://"+lis.Addr().String(),
		http.DefaultClient,
	)

	b.Run("HTTP_GetBook", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			in := &librarypb.GetBookRequest{
				Name: "shelves/1/books/1",
			}
			_, err := ccc.GetBook(ctx, in)
			if err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
	})
	b.Run("HTTP_UpdateBook", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			in := &librarypb.UpdateBookRequest{
				Book: &librarypb.Book{
					Name:  "shelves/1/books/1",
					Title: "The Great Gatsby",
				},
				UpdateMask: &field_mask.FieldMask{
					Paths: []string{"book.title"},
				},
			}
			_, err := ccc.UpdateBook(ctx, in)
			if err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
	})
	b.Run("HTTP_DeleteBook", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			in := &librarypb.DeleteBookRequest{
				Name: "shelves/1/books/1",
			}
			_, err := ccc.DeleteBook(ctx, in)
			if err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
	})
}
