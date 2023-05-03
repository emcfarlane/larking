package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"

	"github.com/twitchtv/twirp"
	"golang.org/x/sync/errgroup"
	"larking.io/benchmarks/api/librarypb"
	"larking.io/larking"
)

func TestTwirp(t *testing.T) {
	ctx := context.Background()
	svc := &testService{}

	mux, err := larking.NewMux()
	if err != nil {
		t.Fatal(err)
	}
	librarypb.RegisterLibraryServiceServer(mux, svc)

	ts, err := larking.NewServer(mux,
		larking.InsecureServerOption(),
		larking.MuxHandleOption("/", "/twirp"),
	)
	if err != nil {
		t.Fatal(err)
	}

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()

	var g errgroup.Group
	defer func() {
		if err := g.Wait(); err != nil {
			t.Fatal(err)
		}
		t.Log("all server shutdown")
	}()
	g.Go(func() (err error) {
		if err := ts.Serve(lis); err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	})
	defer func() {
		t.Log("shutdown server")
		if err := ts.Shutdown(ctx); err != nil {
			t.Fatal(err)
		}
	}()

	{ // proto
		ccproto := librarypb.NewLibraryServiceProtobufClient("http://"+lis.Addr().String(), &http.Client{})

		book, err := ccproto.GetBook(ctx, &librarypb.GetBookRequest{
			Name: "shelves/1/books/1",
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Log(book, err)

		// 404
		if _, err = ccproto.GetBook(ctx, &librarypb.GetBookRequest{
			Name: "shelves/1/books/404",
		}); err == nil {
			t.Fatal("should be 404")
		} else {
			var twerr twirp.Error
			if errors.As(err, &twerr) {
				t.Log(twerr.Code(), twerr.Msg())
				if twerr.Code() != twirp.NotFound {
					t.Errorf("should be %s, but got %s", twirp.NotFound, twerr.Code())
				}
			} else {
				t.Error(err)
			}
		}

		if _, err := ccproto.CreateBook(ctx, &librarypb.CreateBookRequest{
			Parent: "shelves/1",
			Book: &librarypb.Book{
				Name:  "shelves/1/books/2",
				Title: "book2",
			},
		}); err != nil {
			t.Error(err)
		}

		if _, err := ccproto.DeleteBook(ctx, &librarypb.DeleteBookRequest{
			Name: "shelves/1/books/1",
		}); err != nil {
			t.Error(err)
		}
	}

	{ // json
		ccjson := librarypb.NewLibraryServiceJSONClient("http://"+lis.Addr().String(), &http.Client{})
		book, err := ccjson.GetBook(ctx, &librarypb.GetBookRequest{
			Name: "shelves/1/books/1",
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Log(book, err)

		// 404
		if _, err = ccjson.GetBook(ctx, &librarypb.GetBookRequest{
			Name: "shelves/1/books/404",
		}); err == nil {
			t.Fatal("should be 404")
		} else {
			var twerr twirp.Error
			if errors.As(err, &twerr) {
				t.Log(twerr.Code(), twerr.Msg())
				if twerr.Code() != twirp.NotFound {
					t.Errorf("should be %s, but got %s", twirp.NotFound, twerr.Code())
				}
			} else {
				t.Error(err)
			}
		}

		if _, err := ccjson.CreateBook(ctx, &librarypb.CreateBookRequest{
			Parent: "shelves/1",
			Book: &librarypb.Book{
				Name:  "shelves/1/books/2",
				Title: "book2",
			},
		}); err != nil {
			t.Error(err)
		}

		if _, err := ccjson.DeleteBook(ctx, &librarypb.DeleteBookRequest{
			Name: "shelves/1/books/1",
		}); err != nil {
			t.Error(err)
		}
	}
}
