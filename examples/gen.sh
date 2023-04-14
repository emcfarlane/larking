protoc -I ~/src/github.com/googleapis/api-common-protos/ --go_out=module=larking.io/examples:. --go-grpc_out=module=larking.io/examples:. -I. ./api/*.proto
