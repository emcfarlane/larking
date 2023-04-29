protoc -I ~/src/github.com/googleapis/api-common-protos/ -I. \
	--go_out=module=larking.io/benchmarks:. \
	--go-grpc_out=module=larking.io/benchmarks:. \
	--grpc-gateway_out=module=larking.io/benchmarks:. \
	--connect-go_out=module=larking.io/benchmarks:. \
	--twirp_out=module=larking.io/benchmarks:. \
	--include_imports --include_source_info \
	--descriptor_set_out=api/proto.pb \
	./api/*.proto
