package main

import (
	"log"
	"net"
	"net/http"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"larking.io/benchmarks/api/librarypb"
)

//go:generate sh gen.sh

func run() error {
	svc := &testService{}
	gs := grpc.NewServer(grpc.Creds(insecure.NewCredentials()))
	librarypb.RegisterLibraryServiceServer(gs, svc)

	lis, err := net.Listen("tcp", "localhost:5050")
	if err != nil {
		return err
	}
	defer lis.Close()

	log.Printf("listening on %s", lis.Addr().String())
	if err := gs.Serve(lis); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
