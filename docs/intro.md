---
id: intro
title: Getting Started
sidebar_label: Getting Started
---

TODO

---

## Setup

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
