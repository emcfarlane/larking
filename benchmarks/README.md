# Benchmarks

Comparisons against other services. See `bench.txt` for results.

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
