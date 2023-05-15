// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package larking

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
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

// NewServer creates a new http.Server with http2 support.
// The server is configured with the given options.
// It is a convenience function for creating a new http.Server.
func NewServer(mux *Mux, opts ...ServerOption) (*http.Server, error) {
	if mux == nil {
		return nil, fmt.Errorf("invalid mux must not be nil")
	}

	var svrOpts serverOptions
	for _, opt := range opts {
		if err := opt(&svrOpts); err != nil {
			return nil, err
		}
	}

	h := svrOpts.serveMux
	if h == nil {
		h = http.NewServeMux()
	}
	if len(svrOpts.muxPatterns) == 0 {
		svrOpts.muxPatterns = []string{"/"}
	}
	for _, pattern := range svrOpts.muxPatterns {
		prefix := strings.TrimSuffix(pattern, "/")
		if len(prefix) > 0 {
			h.Handle(prefix+"/", http.StripPrefix(prefix, mux))
		} else {
			h.Handle("/", mux)
		}
	}

	h2s := &http2.Server{}
	hs := &http.Server{
		ReadHeaderTimeout: 10 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MB
		Handler:           h2c.NewHandler(h, h2s),
		TLSConfig:         svrOpts.tlsConfig,
	}
	if err := http2.ConfigureServer(hs, h2s); err != nil {
		return nil, err
	}
	return hs, nil
}

type serverOptions struct {
	tlsConfig   *tls.Config
	serveMux    *http.ServeMux
	muxPatterns []string
}

// ServerOption is similar to grpc.ServerOption.
type ServerOption func(*serverOptions) error

func TLSCredsOption(c *tls.Config) ServerOption {
	return func(opts *serverOptions) error {
		opts.tlsConfig = c
		return nil
	}
}

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
