package main

import (
	"log"
	"net"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/genproto/googleapis/api/serviceconfig"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"larking.io/larking"
)

func main() {
	// Create the health service.
	healthSvc := health.NewServer()
	healthSvc.SetServingStatus("example.up.Service", healthpb.HealthCheckResponse_SERVING)
	healthSvc.SetServingStatus("example.down.Service", healthpb.HealthCheckResponse_NOT_SERVING)

	// ServiceConfigOption is used to add extra HTTP annotations to gPRC Methods.
	// This is used to add HTTP endpoints to the HealthServer.
	sc := &serviceconfig.Service{
		Http: &annotations.Http{Rules: []*annotations.HttpRule{{
			// Selector is the gRPC method name.
			Selector: "grpc.health.v1.Health.Check",
			// Pattern is the HTTP pattern to map to.
			Pattern: &annotations.HttpRule_Get{
				// Get is a HTTP GET.
				Get: "/v1/healthz",
			},
		}, {
			// Watch is a gRPC streaming method.
			Selector: "grpc.health.v1.Health.Watch",
			Pattern: &annotations.HttpRule_Custom{
				// Custom is a custom pattern.
				Custom: &annotations.CustomHttpPattern{
					// Kind "WEBSOCKET" is a HTTP WebSocket.
					Kind: "WEBSOCKET",
					// Path is the same as above.
					Path: "/v1/healthz",
				},
			},
		}}},
	}

	// Mux implements http.Handler, use by itself to sever only HTTP endpoints.
	mux, err := larking.NewMux(
		larking.ServiceConfigOption(sc),
	)
	if err != nil {
		log.Fatal(err)
	}
	// RegisterHealthServer registers a HealthServer to the mux.
	healthpb.RegisterHealthServer(mux, healthSvc)

	// Server is a gRPC server that serves both gRPC and HTTP endpoints.
	svr, err := larking.NewServer(mux, larking.InsecureServerOption())
	if err != nil {
		log.Fatal(err)
	}

	// Listen on TCP port 8080 on all interfaces.
	lis, err := net.Listen("tcp", "localhost:8080")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()

	// Serve starts the server and blocks until the server stops.
	// http://localhost:8080/v1/healthz
	log.Println("gRPC & HTTP server listening on", lis.Addr())
	if err := svr.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
