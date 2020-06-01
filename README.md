# [WIP] GraphPB

Reflective gRPC gateway proxy.

- [Transcoding protobuf descriptors REST/HTTP to gRPC](https://cloud.google.com/endpoints/docs/grpc/transcoding)
- [Follows Google API Design principles](https://cloud.google.com/apis/design)
- [Dynamically load descriptors via gRPC server reflection](https://github.com/grpc/grpc/blob/master/doc/server-reflection.md)

### Debugging

Checkout [protobuf](https://github.com/golang/protobuf) at the latest v2 relase.
Go install each protoc generation bin.

Regenerate protoc buffers:

```
graphpb$ protoc -I=. --go_out=:. --go-grpc_out=:. grpc/reflection/v1alpha/*.proto

src$ protoc -I=github.com/googleapis/googleapis -I=. --go_out=. --go-grpc_out=. github.com/emcfarlane/graphpb/testpb/*.proto

bazel run //:gazelle -- update-repos -from_file=go.mod
```
