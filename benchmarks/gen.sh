protoc -I ~/src/github.com/googleapis/api-common-protos/ -I. \
	--go_out=module=larking.io/benchmarks:. \
	--go-grpc_out=module=larking.io/benchmarks:. \
	--go-vtproto_out=module=larking.io/benchmarks:. \
	--go-vtproto_opt=features=marshal+unmarshal+size \
	--grpc-gateway_out=module=larking.io/benchmarks:. \
	--connect-go_out=module=larking.io/benchmarks:. \
	--twirp_out=module=larking.io/benchmarks:. \
	--include_imports --include_source_info \
	--descriptor_set_out=api/proto.pb \
	./api/*.proto
