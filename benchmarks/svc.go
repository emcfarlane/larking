package main

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"larking.io/benchmarks/api/librarypb"
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
