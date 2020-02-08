WIP: graphpb
============

Dynamic GRPC Gateway proxy.


### Debugging

Checkout [protobuf](https://github.com/golang/protobuf) at the latest v2 relase.
Go install each protoc generation bin.

Regenerate protoc buffers:

```
graphpb$ protoc -I=. --go_out=:. google/api/*
graphpb$ protoc -I=. --go_out=:. --go-grpc_out=:. grpc/reflection/v1alpha/*.proto
graphpb$ mockgen github.com/afking/graphpb/testpb MessagingServer > mock_testpb/mock_testpb.go

src$ protoc -I=. -I=github.com/afking/graphpb/ --go_out=. --go-grpc_out=. github.com/afking/graphpb/testpb/*.proto
```
(TODO: simplify protoc/move to bazel)
