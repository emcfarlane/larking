```
   _,
  ( '>   Welcome to larking.io
 / ) )
 /|^^
```
[![Go Reference](https://pkg.go.dev/badge/larking.io.svg)](https://pkg.go.dev/larking.io/larking)

Larking is a [protoreflect](https://pkg.go.dev/google.golang.org/protobuf/reflect/protoreflect) gRPC-transcoding implementation. 
Bind [`google.api.http`](https://github.com/googleapis/googleapis/blob/master/google/api/http.proto) annotations to gRPC services without code generation.
Works with existing go-protobuf generators 
[`protoc-gen-go`](https://pkg.go.dev/google.golang.org/protobuf@v1.30.0/cmd/protoc-gen-go) and 
[`protoc-gen-go-grpc`](https://pkg.go.dev/google.golang.org/grpc/cmd/protoc-gen-go-grpc).
Bind to local services or proxy to other gRPC servers using gRPC server reflection.
Use Google's [API design guide](https://cloud.google.com/apis/design) to design beautiful RESTful APIs with gRPC services.

- Supports [gRPC](https://grpc.io) clients
- Supports [gRPC-transcoding](https://cloud.google.com/endpoints/docs/grpc/transcoding) clients
- Supports [gRPC-web](https://github.com/grpc/grpc-web) clients
- Supports [twirp](https://github.com/twitchtv/twirp) clients
- Proxy gRPC servers with gRPC [server reflection](https://github.com/grpc/grpc/blob/master/doc/server-reflection.md)
- Implicit `/GRPC_SERVICE_FULL_NAME/METHOD_NAME` for all methods
- Websocket streaming with `websocket` kind annotations
- Content streaming with `google.api.HttpBody`
- Streaming support with [SizeCodec](https://github.com/emcfarlane/larking#streaming-codecs)
- Fast with low allocations: see [benchmarks](https://github.com/emcfarlane/larking/tree/main/benchmarks)

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
## Features

Transcoding provides methods to bind gRPC endpoints to HTTP methods.
An example service `Library`:
```protobuf
package larking.example

service Library {
  // GetBook returns a book from a shelf.
  rpc GetBook(GetBookRequest) returns (Book) {};
}
```

Implicit bindings are provided on all methods bound to the URL
`/GRPC_SERVICE_FULL_NAME/METHOD_NAME`.
 
For example to get the book with curl we can send a `POST` request:
```
curl -XPOST http://domain/larking.example.Library/GetBook -d '{"name":"shelves/1/books/2"}
```

We can also use any method with query parameters. The equivalent `GET` request:
```
curl http://domain/larking.example.Library/GetBook?name=shelves/1/books/2
```

As an annotation this syntax would be written as a custom http option:
```protobuf
rpc GetBook(GetBookRequest) returns (Book) {
  option (google.api.http) = {
    custom : {kind : "*" path : "/larking.example.Library/GetBook"}
    body : "*"
  };
};
```

To get better URL semantics lets define a custom annotation:
```protobuf
rpc GetBook(GetBookRequest) returns (Book) {
  option (google.api.http) = {
    get : "/v1/{name=shelves/*/books/*}"
  };
};
```

Now the equivalent `GET` request would be:
```
curl http://domain/v1/shelves/1/books/2
```

See the reference docs for [google.api.HttpRule](https://cloud.google.com/endpoints/docs/grpc-service-config/reference/rpc/google.api#google.api.HttpRule) type for all features.

### Extensions
It aims to be a superset of the gRPC transcoding spec with better support for streaming. The implementation also aims to be simple and fast.
API's should be easy to use and have low overhead.
See the `benchmarks/` for details and comparisons.

#### Arbitrary Content

Send any content type with the protobuf type `google.api.HttpBody`.
Request bodies are unmarshalled from the body with the `ContentType` header.
Response bodies marshal to the body and set the `ContentType` header.
For large requests streaming RPCs support chunking the file into multiple messages.
This can be used as a way to document content APIs.

```protobuf
import "google/api/httpbody.proto";

service Files {
  // HTTP | gRPC
  // -----|-----
  // `POST /files/cat.jpg <body>` | `UploadDownload(filename: "cat.jpg", file:
  // { content_type: "image/jpeg", data: <body>})"`
  rpc UploadDownload(UploadFileRequest) returns (google.api.HttpBody) {
    option (google.api.http) = {
      post : "/files/{filename}"
      body : "file"
    };
  }
  rpc LargeUploadDownload(stream UploadFileRequest)
      returns (stream google.api.HttpBody) {
    option (google.api.http) = {
      post : "/files/large/{filename}"
      body : "file"
    };
  }
}
message UploadFileRequest {
  string filename = 1;
  google.api.HttpBody file = 2;
}
```


#### Websockets Annotations
Annotate a custom method kind `websocket` to enable clients to upgrade connections. This enables streams to be bidirectional over a websocket connection.
```protobuf
// Chatroom shows the websocket extension.
service ChatRoom {
  rpc Chat(stream ChatMessage) returns (stream ChatMessage) {
    option (google.api.http) = {
      custom : {kind : "websocket" path : "/v1/{name=rooms/*}"}
      body : "*"
    };
  }
}
```

#### Streaming Codecs
Streaming requests servers will upgrade the codec interface to read and write 
marshalled messages to the stream. 
This allows codecs to control framing on the wire.
For other protocols like `websockets` framing is controlled by the protocol and this isn't needed. Unlike gRPC encoding where compressions is _per message_, compression is based on the stream so only a method to delimiter messages is required.

For example JSON messages are delimited based on the outer JSON braces `{...}`.
This makes it easy to append payloads to the stream. 
To curl a streaming client method with two messages we can append all the JSON messages in the body:
```
curl http://larking.io/v1/streaming -d '{"message":"one"}{"message":"two"}'
```
This will split into two messages.

Protobuf messages require the length to be encoded before the message `[4 byte length]<binary encoded>`.

