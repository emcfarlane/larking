package main

import (
	"log"
	"net"

	"google.golang.org/genproto/googleapis/api/serviceconfig"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"larking.io/health"
	"larking.io/larking"
)

func main() {
	// Create a health service. The health service is used to check the status
	// of services running within the server.
	healthSvc := health.NewServer()
	healthSvc.SetServingStatus("example.up.Service", healthpb.HealthCheckResponse_SERVING)
	healthSvc.SetServingStatus("example.down.Service", healthpb.HealthCheckResponse_NOT_SERVING)

	serviceConfig := &serviceconfig.Service{}
	// AddHealthz adds a /v1/healthz endpoint to the service binding to the
	// grpc.health.v1.Health service:
	//   - get /v1/healthz -> grpc.health.v1.Health.Check
	//   - websocket /v1/healthz -> grpc.health.v1.Health.Watch
	health.AddHealthz(serviceConfig)

	// Mux impements http.Handler and serves both gRPC and HTTP connections.
	mux, err := larking.NewMux(
		larking.ServiceConfigOption(serviceConfig),
	)
	if err != nil {
		log.Fatal(err)
	}
	// RegisterHealthServer registers a HealthServer to the mux.
	healthpb.RegisterHealthServer(mux, healthSvc)

	// Server creates a *http.Server.
	svr, err := larking.NewServer(mux)
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
