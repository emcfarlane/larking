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
	"runtime"
	"time"

	"github.com/soheilhy/cmux"
	"golang.org/x/net/trace"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type Server struct {
	opts serverOptions
	mux  Mux

	closer chan bool
	gs     *grpc.Server
	hs     *http.Server

	events trace.EventLog
}

// NewServer creates a new Proxy server.
func NewServer(opts ...ServerOption) (*Server, error) {
	var svrOpts = defaultServerOptions
	for _, opt := range opts {
		opt(&svrOpts)
	}

	// TODO: use our own flag?
	// grpc.EnableTracing sets tracing for the golang.org/x/net/trace
	var events trace.EventLog
	if grpc.EnableTracing {
		_, file, line, _ := runtime.Caller(1)
		events = trace.NewEventLog("larking.Server", fmt.Sprintf("%s:%d", file, line))
	}

	return &Server{
		opts: svrOpts,
		mux: Mux{
			opts:   svrOpts.muxOpts,
			events: events,
		},
		events: events,
	}, nil
}

func (s *Server) Mux() *Mux {
	return &s.mux
}

func (s *Server) grpcOpts() []grpc.ServerOption {
	var grpcOpts []grpc.ServerOption

	grpcOpts = append(grpcOpts, grpc.UnknownServiceHandler(s.Mux().StreamHandler()))

	if c := s.opts.tlsConfig; c != nil {
		creds := credentials.NewTLS(c)
		grpcOpts = append(grpcOpts, grpc.Creds(creds))
	}

	return grpcOpts
}

func (s *Server) Serve(l net.Listener) error {
	s.closer = make(chan bool, 1)

	m := cmux.New(l)
	// grpcL := m.Match(cmux.HTTP2HeaderField("content-type", "application/grpc"))
	// gRPC client blocks until it receives a SETTINGS frame from the server.
	grpcL := m.MatchWithWriters(
		cmux.HTTP2MatchHeaderFieldSendSettings("content-type", "application/grpc"),
	)
	httpL := m.Match(cmux.Any())

	n := 3
	errs := make(chan error, n)

	gs := grpc.NewServer(s.grpcOpts()...)
	go func() { errs <- gs.Serve(grpcL) }()
	defer gs.Stop()

	hs := &http.Server{
		Handler:   &s.mux,
		TLSConfig: s.opts.tlsConfig,
	}
	go func() {
		if s.opts.tlsConfig != nil {
			// TLSConfig must have the cert object.
			errs <- hs.ServeTLS(httpL, "", "")
		} else {
			errs <- hs.Serve(httpL)
		}
	}()
	defer hs.Close()

	// TODO: metrics/debug http server...?

	go func() { errs <- m.Serve() }()

	s.gs = gs
	s.hs = hs

	select {
	case <-s.closer:
		return nil

	case err := <-errs:
		return err
	}
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.closer == nil {
		return nil
	}

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return s.hs.Shutdown(ctx)
	})
	g.Go(func() error {
		s.gs.GracefulStop()
		return nil
	})
	if err := g.Wait(); err != nil {
		return err
	}
	if s.events != nil {
		s.events.Finish()
		s.events = nil
	}
	return s.Close()
}

func (s *Server) Close() error {
	if s.closer != nil {
		close(s.closer)
	}
	return nil
}

const (
	defaultServerMaxReceiveMessageSize = 1024 * 1024 * 4
	defaultServerMaxSendMessageSize    = math.MaxInt32
	defaultServerConnectionTimeout     = 120 * time.Second
)

type serverOptions struct {
	tlsConfig *tls.Config
	muxOpts   muxOptions
	//admin     string
}

var defaultServerOptions = serverOptions{
	muxOpts: defaultMuxOptions,
}

// ServerOption is similar to grpc.ServerOption.
type ServerOption func(*serverOptions)

func TLSCreds(c *tls.Config) ServerOption {
	return func(opts *serverOptions) {
		opts.tlsConfig = c
	}
}

func MuxOptions(muxOpts ...MuxOption) ServerOption {
	return func(opts *serverOptions) {
		for _, mo := range muxOpts {
			mo(&opts.muxOpts)
		}
	}
}

//func AdminOption(addr string) ServerOption {
//	return func(opts *serverOptions) {
//
//	}
//}
