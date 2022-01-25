// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package larking

import (
	"context"
	"crypto/tls"
	"fmt"
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/soheilhy/cmux"
	"golang.org/x/net/http2"
	"golang.org/x/net/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// NewOSSignalContext tries to gracefully handle OS closure.
func NewOSSignalContext(ctx context.Context) (context.Context, func()) {
	// trap Ctrl+C and call cancel on the context
	ctx, cancel := context.WithCancel(ctx)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		select {
		case <-c:
			cancel()
		case <-ctx.Done():
		}
	}()

	return ctx, func() {
		signal.Stop(c)
		cancel()
	}
}

type Server struct {
	opts serverOptions
	mux  *Mux

	closer chan bool
	gs     *grpc.Server
	hs     *http.Server

	events trace.EventLog
}

// NewServer creates a new Proxy server.
func NewServer(mux *Mux, opts ...ServerOption) (*Server, error) {
	if mux == nil {
		return nil, fmt.Errorf("invalid mux must not be nil")
	}

	var svrOpts serverOptions
	for _, opt := range opts {
		if err := opt(&svrOpts); err != nil {
			return nil, err
		}
	}
	svrOpts.serveMux = http.NewServeMux()
	if len(svrOpts.muxPatterns) == 0 {
		svrOpts.muxPatterns = []string{"/"}
	}
	for _, pattern := range svrOpts.muxPatterns {
		svrOpts.serveMux.Handle(pattern, mux)
	}

	// TODO: use our own flag?
	// grpc.EnableTracing sets tracing for the golang.org/x/net/trace
	var events trace.EventLog
	if grpc.EnableTracing {
		_, file, line, _ := runtime.Caller(1)
		events = trace.NewEventLog("larking.Server", fmt.Sprintf("%s:%d", file, line))
	}

	var grpcOpts []grpc.ServerOption

	grpcOpts = append(grpcOpts, grpc.UnknownServiceHandler(mux.StreamHandler()))
	if i := mux.opts.unaryInterceptor; i != nil {
		grpcOpts = append(grpcOpts, grpc.UnaryInterceptor(i))
	}
	if i := mux.opts.streamInterceptor; i != nil {
		grpcOpts = append(grpcOpts, grpc.StreamInterceptor(i))
	}

	creds := insecure.NewCredentials()
	if config := svrOpts.tlsConfig; config != nil {
		creds = credentials.NewTLS(config)
	} else if !svrOpts.insecure {
		return nil, fmt.Errorf("credentials must be set")
	}
	grpcOpts = append(grpcOpts, grpc.Creds(creds))

	gs := grpc.NewServer(grpcOpts...)
	hs := &http.Server{
		Handler:   svrOpts.serveMux,
		TLSConfig: svrOpts.tlsConfig,
	}

	// Register local gRPC services
	for sd, ss := range mux.services {
		gs.RegisterService(sd, ss)
	}

	//if s.healthServer != nil {
	//	s.healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	//	s.RegisterService(&healthpb.Health_ServiceDesc, s.healthServer)
	//}

	return &Server{
		opts:   svrOpts,
		mux:    mux,
		gs:     gs,
		hs:     hs,
		events: events,
	}, nil
}

func (s *Server) Serve(l net.Listener) error {
	s.closer = make(chan bool, 1)

	if config := s.opts.tlsConfig; config != nil {
		l = tls.NewListener(l, config)

		// TODO: needed?
		if err := http2.ConfigureServer(s.hs, nil); err != nil {
			return err
		}
	}

	m := cmux.New(l)
	// grpcL := m.Match(cmux.HTTP2HeaderField("content-type", "application/grpc"))
	// gRPC client blocks until it receives a SETTINGS frame from the server.
	grpcL := m.MatchWithWriters(
		cmux.HTTP2MatchHeaderFieldSendSettings("content-type", "application/grpc"),
	)
	httpL := m.Match(cmux.Any())

	n := 3
	errs := make(chan error, n)

	go func() { errs <- s.gs.Serve(grpcL) }()
	defer s.gs.Stop()

	go func() {
		errs <- s.hs.Serve(httpL)
		//if s.opts.tlsConfig != nil {
		//	// TLSConfig must have the cert object.
		//       errs <- s.hs.ServeTLS(httpL, "", "")
		//} else {
		//}
	}()
	defer s.hs.Close()

	go func() {
		if err := m.Serve(); !strings.Contains(err.Error(), "use of closed") {
			errs <- err
		}

	}()

	select {
	case <-s.closer:
		return http.ErrServerClosed

	case err := <-errs:
		return err
	}
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.events != nil {
		s.events.Finish()
		s.events = nil
	}
	if s.closer == nil {
		return nil
	}
	close(s.closer)
	s.gs.GracefulStop()
	if err := s.hs.Shutdown(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Server) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.Shutdown(ctx)
}

const (
	defaultServerMaxReceiveMessageSize = 1024 * 1024 * 4
	defaultServerMaxSendMessageSize    = math.MaxInt32
	defaultServerConnectionTimeout     = 120 * time.Second
)

type serverOptions struct {
	tlsConfig *tls.Config
	insecure  bool
	log       logr.Logger

	//mux      *Mux
	muxPatterns []string
	serveMux    *http.ServeMux

	//muxOpts muxOptions
	//admin     string
}

// ServerOption is similar to grpc.ServerOption.
type ServerOption func(*serverOptions) error

func TLSCredsOption(c *tls.Config) ServerOption {
	return func(opts *serverOptions) error {
		opts.tlsConfig = c
		return nil
	}
}

func InsecureServerOption() ServerOption {
	return func(opts *serverOptions) error {
		opts.insecure = true
		return nil
	}
}

//func LarkingServerOption(threads map[string]string) ServerOption {
//	return func(opts *serverOptions) error {
//		opts.larkingEnabled = true
//		opts.larkingThreads = threads
//		return nil
//	}
//}

func LogOption(log logr.Logger) ServerOption {
	return func(opts *serverOptions) error {
		opts.log = log
		return nil
	}
}

//func MuxOptions(muxOpts ...MuxOption) ServerOption {
//	return func(opts *serverOptions) error {
//		for _, mo := range muxOpts {
//			mo(&opts.muxOpts)
//		}
//		return nil
//	}
//}

func MuxHandleOption(patterns ...string) ServerOption {
	return func(opts *serverOptions) error {
		if opts.muxPatterns != nil {
			return fmt.Errorf("more than one mux registered")
		}
		opts.muxPatterns = patterns
		return nil
	}
}

func HTTPHandlerOption(pattern string, handler http.Handler) ServerOption {
	return func(opts *serverOptions) error {
		if opts.serveMux == nil {
			opts.serveMux = http.NewServeMux()
		}
		opts.serveMux.Handle(pattern, handler)
		return nil
	}
}

//func AdminOption(addr string) ServerOption {
//	return func(opts *serverOptions) {
//
//	}
//}

//func (s *Server) RegisterService(desc *grpc.ServiceDesc, impl interface{}) {
//	s.gs.RegisterService(desc, impl)
//	if s.opts.mux != nil {
//		s.opts.mux.RegisterService(desc, impl)
//	}
//}
