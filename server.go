package graphpb

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/soheilhy/cmux"
	"google.golang.org/grpc"
)

type Server struct {
	mux    *Mux
	closer chan struct{}
	// TODO: server options, maybe maps to gRPC options
}

func NewServer(ctx context.Context, services []string) (*Server, error) {
	var ccs []*grpc.ClientConn
	for _, svc := range services {
		cc, err := grpc.DialContext(
			ctx, svc, grpc.WithInsecure(),
		) // TODO: options config
		if err != nil {
			return nil, err
		}
		ccs = append(ccs, cc)
	}

	m, err := NewMux(ccs...)
	if err != nil {
		return nil, err
	}

	return &Server{
		mux:    m,
		closer: make(chan struct{}),
	}, nil
}

func (s *Server) Serve(l net.Listener) error {
	m := cmux.New(l)

	grpcL := m.Match(cmux.HTTP2HeaderField("content-type", "application/grpc"))
	httpL := m.Match(cmux.Any())

	n := 3
	errs := make(chan error, n)

	gs := grpc.NewServer(
		grpc.UnknownServiceHandler(s.mux.StreamHandler()),
	)
	go func() { errs <- gs.Serve(grpcL) }()
	defer gs.Stop()

	hs := &http.Server{
		Handler: s.mux,
	}
	go func() { errs <- hs.Serve(httpL) }()
	defer hs.Close()

	// TODO: metrics/debug http server...?

	go func() { errs <- m.Serve() }()

	fmt.Println("listening", l.Addr().String())
	select {
	case <-s.closer:
		return nil
	case err := <-errs:
		return err
	}
}

func (s *Server) Close() error {
	if s.closer != nil {
		close(s.closer)
	}
	return nil
}
