WIP: GRPC Gateway
============

Dynamic GRPC Gateway proxy.


### Debugging

Regenerate protoc buffers:

```
gateway$ protoc -I=. --go_out=google/api google/api/*

src$ protoc -I=. -I=github.com/afking/gateway/ --go_out=. --go-grpc_out=. github.com/afking/gateway/testpb/*.proto
```
