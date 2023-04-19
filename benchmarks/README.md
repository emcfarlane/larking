# Benchmarks

Comparisons against other services. See `bench.txt` for results.

## gRPC-Gateway

[gRPC-Gateway](https://github.com/grpc-ecosystem/grpc-gateway)
generated `api/librarypb/library.pb.gw.go` file to proxy JSON requests.

## Envoy

[gRPC-JSON transcoder](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/grpc_json_transcoder_filter) with configuration in testdata/envoy.yaml.
Slower than both larking and gRPC-Gateway, price for proxying requests.
