protoc -I ~/src/github.com/googleapis/api-common-protos/ -I. \
	--go_out=module=larking.io/benchmarks:. \
	--go-grpc_out=module=larking.io/benchmarks:. \
	--grpc-gateway_out=module=larking.io/benchmarks:. \
	./api/*.proto
