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
	"strings"
	"time"

	"github.com/soheilhy/cmux"
	"golang.org/x/net/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type Server struct {
	opts serverOptions
	mux  *Mux

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

	mux := &Mux{
		opts:   svrOpts.muxOpts,
		events: events,
	}

	var grpcOpts []grpc.ServerOption
	grpcOpts = append(grpcOpts, grpc.UnknownServiceHandler(mux.StreamHandler()))
	if c := svrOpts.tlsConfig; c != nil {
		creds := credentials.NewTLS(c)
		grpcOpts = append(grpcOpts, grpc.Creds(creds))
	}

	gs := grpc.NewServer(grpcOpts...)
	hs := &http.Server{
		Handler:   mux,
		TLSConfig: svrOpts.tlsConfig,
	}

	return &Server{
		opts:   svrOpts,
		mux:    mux,
		gs:     gs,
		hs:     hs,
		events: events,
	}, nil
}

func (s *Server) Mux() *Mux { return s.mux }

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

	go func() { errs <- s.gs.Serve(grpcL) }()
	defer s.gs.Stop()

	go func() {
		if s.opts.tlsConfig != nil {
			// TLSConfig must have the cert object.
			errs <- s.hs.ServeTLS(httpL, "", "")
		} else {
			errs <- s.hs.Serve(httpL)
		}
	}()
	defer s.hs.Close()

	// TODO: metrics/debug http server...?

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

func (s *Server) RegisterService(desc *grpc.ServiceDesc, impl interface{}) {
	s.gs.RegisterService(desc, impl)
	s.mux.RegisterService(desc, impl)
}
