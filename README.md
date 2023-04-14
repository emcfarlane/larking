```
   _,
  ( '>   Welcome to larking.io
 / ) )
 /|^^
```
[![Go Reference](https://pkg.go.dev/badge/larking.io.svg)](https://pkg.go.dev/larking.io/larking)

Larking is a reflective gRPC transcoding handler. Easily serve REST api's from gRPC services. Proxy other language servers using gRPC reflection. See the examples for details!

- [Transcoding protobuf descriptors REST/HTTP to gRPC](https://cloud.google.com/endpoints/docs/grpc/transcoding)
- [Follows Google API Design principles](https://cloud.google.com/apis/design)
- [Dynamically load descriptors via gRPC server reflection](https://github.com/grpc/grpc/blob/master/doc/server-reflection.md)

<div align="center">
<img src="docs/larking.svg" />
</div>

## Install

```
go get larking.io@latest
```

## Quickstart

Compile protobuffers with go and go-grpc libraries. Follow the guide [here](https://grpc.io/docs/languages/go/quickstart/#prerequisites). No other pre-compiled libraries are required. We need to create a `larking.Mux` and optionally `larking.Server` to serve both gRPC and REST. 

This example builds a server with the health service:

```go
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
```

