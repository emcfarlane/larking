WIP: GRPC Gateway
============

Dynamic GRPC Gateway proxy.


### Debugging

Checkout [protobuf](https://github.com/golang/protobuf) at the latest v2 relase.
Go install each protoc generation bin.

Regenerate protoc buffers:

```
gateway$ protoc -I=. --go_out=google/api google/api/*
gateway$ protoc -I=. --go_out=:. grpc/reflection/v1alpha/*.proto

src$ protoc -I=. -I=github.com/afking/gateway/ --go_out=. --go-grpc_out=. github.com/afking/gateway/testpb/*.proto
```
