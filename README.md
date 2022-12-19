# [larking.io](https://larking.io)

[![Go Reference](https://pkg.go.dev/badge/larking.io.svg)](https://pkg.go.dev/larking.io)

Reflective gRPC transcoding handler. Get started: [larking.io/docs](https://larking.io/docs)

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

### Install the REPL

```
go install larking.io/cmd/lark@latest
```

### Install the worker

```
go install larking.io/cmd/larking@latest
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
mux.RegisterService(&apipb.MyService_ServiceDesc, s) // S is your implementation
```

Create a server and serve gRPC and REST:
```
svr, _ := larking.NewServer(mux, larking.InsecureServerOption())
l, _ := net.Listen("tcp", fmt.Sprintf(":%s", *flagPort))
log.Printf("listening on %s", l.Addr().String())
svr.Serve(l)
```

## Debugging

Checkout [protobuf](https://github.com/golang/protobuf) at the latest v2 relase.
Go install each protoc generation bin.

Regenerate protoc buffers:

```
protoc --go_out=paths=source_relative:. --go-grpc_out=paths=source_relative:. testpb/test.proto
```

### Protoc

Must have googleapis protos avaliable.
Just link API to `/usr/local/include/google` so protoc can find it.
```
ln -s ~/src/github.com/googleapis/googleapis/google/api/ /usr/local/include/google/
```
