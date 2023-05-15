# Benchmarks

Comparisons against other popular services. See `bench.txt` for results.
Benchmarks should be taken with a grain of salt, please add more if you'd like to expand the test cases!

## Larking
Larking serves both JSON and Protobuf encoded requests, tests marked with `+pb` are protobuf encoded.

### Optimisations
- https://www.emcfarlane.com/blog/2023-04-18-profile-lexer
- https://www.emcfarlane.com/blog/2023-05-01-bufferless-append

## gRPC-Gateway

[gRPC-Gateway](https://github.com/grpc-ecosystem/grpc-gateway)
generated `api/librarypb/library.pb.gw.go` file to proxy JSON requests.

## Envoy

[gRPC-JSON transcoder](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/grpc_json_transcoder_filter) with configuration in testdata/envoy.yaml.
Slower than both larking and gRPC-Gateway, price for proxying requests.

## Gorilla Mux

[Gorilla Mux](https://github.com/gorilla/mux) is a popular routing library for HTTP services in Go.
Here we write a custom mux that replicates the gRPC annotations and binds the mux to the gRPC server.
Compares speed with writing the annotations binding by hand, useful for compairsons with Go routing libraries.

## Connect-go

[Connect-go](https://github.com/bufbuild/connect-go) is a slim library with gRPC compatible HTTP APIs.

## Twirp

[Twirp](https://github.com/twitchtv/twirp) is a simple RPC protocol based on HTTP and Protocol Buffers (proto).


## gRPC

Compare gRPC server benchmarks with the `mux.ServeHTTP`.
We use an altered version of go-gRPC's [benchmain](https://github.com/grpc/grpc-go/blob/master/Documentation/benchmark.md)
tool to run a benchmark and compare it to gRPC internal server.

```
go run benchmain/main.go -benchtime=10s -workloads=all \
          -compression=gzip -maxConcurrentCalls=1 -trace=off \
          -reqSizeBytes=1,1048576 -respSizeBytes=1,1048576 -networkMode=Local \
          -cpuProfile=cpuProf -memProfile=memProf -memProfileRate=10000 -resultFile=result.bin
```

```
go run google.golang.org/grpc/benchmark/benchresult grpc_result.bin result.bin
```
