module larking.io/benchmarks

go 1.20

require (
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.15.2
	github.com/soheilhy/cmux v0.1.5
	golang.org/x/sync v0.1.0
	google.golang.org/genproto v0.0.0-20230410155749-daa745c078e1
	google.golang.org/grpc v1.54.0
	google.golang.org/protobuf v1.30.0
	larking.io v0.0.0-20230415140254-4fbc95c206cd
)

require (
	github.com/gobwas/httphead v0.1.0 // indirect
	github.com/gobwas/pool v0.2.1 // indirect
	github.com/gobwas/ws v1.2.0 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/gorilla/mux v1.8.0 // indirect
	golang.org/x/net v0.9.0 // indirect
	golang.org/x/sys v0.7.0 // indirect
	golang.org/x/text v0.9.0 // indirect
)

replace larking.io => ../
