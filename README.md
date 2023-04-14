```
   _,
  ( '>   Welcome to larking.io
 / ) )
 /|^^
```
[![Go Reference](https://pkg.go.dev/badge/larking.io.svg)](https://pkg.go.dev/larking.io)

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

Compile protobuffers to Go:
```
protoc --go_out=module=larking.io:. --go-grpc_out=module=larking.io:. api/*.proto
```

Create a `larking.Mux`:
```
mux, _ := larking.NewMux()
```

Register services:
```
mux.RegisterService(&apipb.MyService_ServiceDesc, s) // s is your implementation
```

Create a server and serve gRPC and REST:
```
svr, _ := larking.NewServer(mux, larking.InsecureServerOption())
l, _ := net.Listen("tcp", fmt.Sprintf(":%s", *flagPort))
log.Printf("listening on %s", l.Addr().String())
svr.Serve(l)
```
