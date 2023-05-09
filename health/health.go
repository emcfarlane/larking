// package health provides gRPC health server HTTP annotations.
package health

import (
	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/genproto/googleapis/api/serviceconfig"
	"google.golang.org/grpc/health"
	"google.golang.org/protobuf/proto"
)

// AddHealthz adds a /v1/healthz endpoint to the service binding to the
// grpc.health.v1.Health service:
//   - get /v1/healthz -> grpc.health.v1.Health.Check
//   - websocket /v1/healthz -> grpc.health.v1.Health.Watch
func AddHealthz(dst *serviceconfig.Service) {
	src := &serviceconfig.Service{
		Http: &annotations.Http{Rules: []*annotations.HttpRule{{
			Selector: "grpc.health.v1.Health.Check",
			Pattern: &annotations.HttpRule_Get{
				// Get is a HTTP GET.
				Get: "/v1/healthz",
			},
		}, {
			Selector: "grpc.health.v1.Health.Watch",
			Pattern: &annotations.HttpRule_Custom{
				Custom: &annotations.CustomHttpPattern{
					Kind: "WEBSOCKET",
					Path: "/v1/healthz",
				},
			},
		}}},
	}
	proto.Merge(dst, src)
}

// NewServer returns a new grpc.Health server.
func NewServer() *health.Server { return health.NewServer() }
