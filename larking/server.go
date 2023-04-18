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

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"golang.org/x/net/trace"
	"google.golang.org/grpc"
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

	gs  *grpc.Server
	hs  *http.Server
	h2s *http2.Server

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
	if svrOpts.tlsConfig == nil && !svrOpts.insecure {
		return nil, fmt.Errorf("credentials must be set")
	}

	svrOpts.serveMux = http.NewServeMux()
	if len(svrOpts.muxPatterns) == 0 {
		svrOpts.muxPatterns = []string{"/"}
	}
	for _, pattern := range svrOpts.muxPatterns {
		prefix := strings.TrimSuffix(pattern, "/")
		if len(prefix) > 0 {
			svrOpts.serveMux.Handle(prefix+"/", http.StripPrefix(prefix, mux))
		} else {
			svrOpts.serveMux.Handle("/", mux)
		}
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
	if h := mux.opts.statsHandler; h != nil {
		grpcOpts = append(grpcOpts, grpc.StatsHandler(h))
	}

	// TLS termination controlled by listeners in Serve.
	creds := insecure.NewCredentials()
	grpcOpts = append(grpcOpts, grpc.Creds(creds))

	gs := grpc.NewServer(grpcOpts...)
	// Register local gRPC services
	for sd, ss := range mux.services {
		gs.RegisterService(sd, ss)
	}
	serveWeb := createGRPCWebHandler(gs)
	index := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType := r.Header.Get("content-type")
		if strings.HasPrefix(contentType, grpcWeb) {
			serveWeb(w, r)
		} else if r.ProtoMajor == 2 && strings.HasPrefix(contentType, grpcBase) {
			gs.ServeHTTP(w, r)
		} else {
			svrOpts.serveMux.ServeHTTP(w, r)
		}
	})
	h2s := &http2.Server{}
	hs := &http.Server{
		Handler:   h2c.NewHandler(index, h2s),
		TLSConfig: svrOpts.tlsConfig,
	}
	if err := http2.ConfigureServer(hs, h2s); err != nil {
		return nil, err
	}

	return &Server{
		opts:   svrOpts,
		mux:    mux,
		gs:     gs,
		hs:     hs,
		h2s:    h2s,
		events: events,
	}, nil
}

// Serve accepts incoming connections on the listener.
// Serve will return always return a non-nil error, http.ErrServerClosed.
func (s *Server) Serve(l net.Listener) error {
	if config := s.opts.tlsConfig; config != nil {
		l = tls.NewListener(l, config)
	}
	return s.hs.Serve(l)
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.events != nil {
		s.events.Finish()
		s.events = nil
	}
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
	tlsConfig   *tls.Config
	serveMux    *http.ServeMux
	muxPatterns []string
	insecure    bool
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

func MuxHandleOption(patterns ...string) ServerOption {
	return func(opts *serverOptions) error {
		if opts.muxPatterns != nil {
			return fmt.Errorf("duplicate mux patterns registered")
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
