# [WIP] larking

Reflective gRPC transcoding handler.

- [Transcoding protobuf descriptors REST/HTTP to gRPC](https://cloud.google.com/endpoints/docs/grpc/transcoding)
- [Follows Google API Design principles](https://cloud.google.com/apis/design)
- [Dynamically load descriptors via gRPC server reflection](https://github.com/grpc/grpc/blob/master/doc/server-reflection.md)

<div align="center">
<img src="docs/larking.svg" />
</div>

## Install

```
go install github.com/emcfarlane/larking
```

## Annotations

gRPC transcoding annotations are added to the `.proto` file. Example:
```c
syntax = "proto3";

package books;

import "google/api/annotations.proto";

service Books {
  // Lists books in a shelf.
  rpc ListBooks(ListBooksRequest) returns (ListBooksResponse) {
    // List method maps to HTTP GET.
    option (google.api.http) = {
      // The `parent` captures the parent resource name, such as "shelves/shelf1".
      get: "/v1/{parent=shelves/*}/books"
    };
  }
}

message ListBooksRequest {
  // The parent resource name, for example, "shelves/shelf1".
  string parent = 1;

  // The maximum number of items to return.
  int32 page_size = 2;

  // The next_page_token value returned from a previous List request, if any.
  string page_token = 3;
}

message ListBooksResponse {
  // The field name should match the noun "books" in the method name.  There
  // will be a maximum number of items returned based on the page_size field
  // in the request.
  repeated Book books = 1;

  // Token to retrieve the next page of results, or empty if there are no
  // more results in the list.
  string next_page_token = 2;
}
```

A great resource for designing resourceful API is google's [API Design Guide](https://cloud.google.com/apis/design).
The proto rules are described in the [`google/api/http.proto`](https://github.com/googleapis/googleapis/blob/master/google/api/http.proto).

## Setup

### Mux

The simpliest setup is to create a http mux, `larking.Handler`, and serve it as a http endpoint.
```go
m := larking.NewMux(
  UnaryServerInterceptorOption(
    func(
      ctx context.Context,
      req interface{},
      info *grpc.UnaryServerInfo,
      handler grpc.UnaryHandler,
    ) (resp interface{}, err error) {
      fmt.Println(info.FullMethod) // prints the called gRPC method name
      return handler(ctx, req)
    },
  )
)

// Register your proto services by name.
if err := m.RegisterServiceByName("google.longrunning.Operations", s); err != nil {
  return err
}
http.ListenAndServe(":8000", hd)
```

To serve both gRPC and HTTP on the same point you need to use a library like `cmux`:
```go
m := cmux.New(l)

grpcL := m.Match(cmux.HTTP2HeaderField("content-type", "application/grpc"))
httpL := m.Match(cmux.Any())

gs := grpc.NewServer(grpc.UnaryInterceptor(s.unaryInterceptor))
longrunning.RegisterOperationsServer(gs, s)
if err := hd.RegisterServiceByName("google.longrunning.Operations", s); err != nil {
  return err
}

mux := http.NewServeMux()
mux.Handle("/", m)

errs := make(chan error, 3)

go func() { errs <- gs.Serve(grpcL) }()
defer gs.Stop()

go func() { errs <- hs.Serve(httpL) }()
defer hs.Close()

go func() { errs <- m.Serve() }()

fmt.Println("listening", l.Addr().String())
select {
case err := <-errs:
  return err
case _ = <-sigChan:
  return nil
}
```

### Proxy

(TODO)

### REPL

REPLs are awesome. So we have a starlark one with gRPC call support.
Use it to get command line interactivity with your services or write scripts.
Easily extendable for custom starlark commands and integrations.
Works through larking or any gRPC service with reflection support.

```
go install github.com/emcfarlane/larking/cmd/lark
```

Now run `lark`:
```
$ lark
>>> # dial the gRPC server
>>> grpc.dial("0.0.0.0:50051")
>>>
>>> # creata a handle to the service (helloworld.Greeter)
>>> s = grpc.service("helloworld.Greeter")
grpc.service helloworld.Greeter
>>>
>>> # send a message
>>> s.SayHello(name = "edward!")
HelloReply(message = "Hello edward!")
```

Protobuffer support is provided by [starlarkproto](https://github.com/emcfarlane/starlarkproto).

## Developing

### Debugging

Checkout [protobuf](https://github.com/golang/protobuf) at the latest v2 relase.
Go install each protoc generation bin.

Regenerate protoc buffers:

```
larking$ protoc -I=. --go_out=:. --go-grpc_out=:. grpc/reflection/v1alpha/*.proto

protoc --go_out=paths=source_relative:. --go-grpc_out=paths=source_relative:. testpb/test.proto

bazel run //:gazelle -- update-repos -from_file=go.mod
```

### Protoc

Just link API to `/usr/local/include/google` so protoc can find it.
```
ln -s ~/src/github.com/googleapis/googleapis/google/api/ /usr/local/include/google/
```

