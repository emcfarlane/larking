package main

import (
	"context"
	"time"

	connect_go "github.com/bufbuild/connect-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"larking.io/benchmarks/api/librarypb"
	cpb "larking.io/benchmarks/api/librarypb/librarypbconnect"
)

type testConnectService struct {
	cpb.UnimplementedLibraryServiceHandler
}

func (testConnectService) GetBook(ctx context.Context, req *connect_go.Request[librarypb.GetBookRequest]) (*connect_go.Response[librarypb.Book], error) {
	if req.Msg.Name != "shelves/1/books/1" {
		return nil, grpc.Errorf(codes.NotFound, "not found")
	}
	return connect_go.NewResponse(&librarypb.Book{
		Name:        "shelves/1/books/1",
		Title:       "The Great Gatsby",
		Author:      "F. Scott Fitzgerald",
		PageCount:   180,
		PublishTime: timestamppb.New(time.Now()),
		Duration:    durationpb.New(1 * time.Hour),
		Price:       wrapperspb.Double(9.99),
	}), nil
}

func (testConnectService) ListBooks(ctx context.Context, req *connect_go.Request[librarypb.ListBooksRequest]) (*connect_go.Response[librarypb.ListBooksResponse], error) {
	if req.Msg.Parent != "shelves/1" {
		return nil, grpc.Errorf(codes.NotFound, "not found")
	}
	return connect_go.NewResponse(&librarypb.ListBooksResponse{
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
	}), nil
}

func (testConnectService) CreateBook(ctx context.Context, req *connect_go.Request[librarypb.CreateBookRequest]) (*connect_go.Response[librarypb.Book], error) {
	return connect_go.NewResponse(req.Msg.Book), nil
}
func (testConnectService) UpdateBook(ctx context.Context, req *connect_go.Request[librarypb.UpdateBookRequest]) (*connect_go.Response[librarypb.Book], error) {
	if req.Msg.Book.GetName() != "shelves/1/books/1" {
		return nil, grpc.Errorf(codes.NotFound, "not found")
	}
	if req.Msg.UpdateMask.Paths[0] != "book.title" {
		return nil, grpc.Errorf(codes.InvalidArgument, "invalid field mask")
	}
	return connect_go.NewResponse(req.Msg.Book), nil
}
func (testConnectService) DeleteBook(ctx context.Context, req *connect_go.Request[librarypb.DeleteBookRequest]) (*connect_go.Response[emptypb.Empty], error) {
	if req.Msg.Name != "shelves/1/books/1" {
		return nil, grpc.Errorf(codes.NotFound, "not found")
	}
	return connect_go.NewResponse(&emptypb.Empty{}), nil
}
