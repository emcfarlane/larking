package main

import (
	"log"
	"net"

	"larking.io/api/healthpb"
	"larking.io/health"
	"larking.io/larking"
)

func main() {
	healthSvc := health.NewServer()

	// Mux implements http.Handler, use by itself to sever only HTTP endpoints.
	mux, err := larking.NewMux()
	if err != nil {
		log.Fatal(err)
	}
	healthpb.RegisterHealthServer(mux, healthSvc)

	// Listen on TCP port 8080 on all interfaces.
	lis, err := net.Listen("tcp", "localhost:8080")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()

	// ServerOption is used to configure the gRPC server.
	svr, err := larking.NewServer(mux, larking.InsecureServerOption())
	if err != nil {
		log.Fatal(err)
	}

	// Serve starts the gRPC server and blocks until the server stops.
	// http://localhost:8080/v1/health
	log.Println("gRPC & HTTP server listening on", lis.Addr())
	if err := svr.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
