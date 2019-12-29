WIP: GRPC Gateway
============

Dynamic GRPC Gateway proxy.


### Debugging

Regenerate protoc buffers:
```
protoc -I=. --go_out=google/api google/api/*
```
