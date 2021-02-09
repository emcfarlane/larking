package graphpb

import (
	"crypto/tls"
	"fmt"
	"math"
	"net"
	"net/http"
	"time"

	"github.com/soheilhy/cmux"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type Server struct {
	opts     *serverOptions
	grpcOpts []grpc.ServerOption

	mux    *Mux
	closer chan struct{}
}

//func NewServer(ctx context.Context, services []string) (*Server, error) {
func NewServer(opts ...ServerOption) (*Server, error) {
	//var ccs []*grpc.ClientConn
	//for _, svc := range services {
	//	cc, err := grpc.DialContext(
	//		ctx, svc, grpc.WithInsecure(),
	//	) // TODO: options config
	//	if err != nil {
	//		return nil, err
	//	}
	//	ccs = append(ccs, cc)
	//}
	// Default
	var grpcOpts []grpc.ServerOption
	svrOpts := &defaultServerOptions
	for _, opt := range opts {
		if grpcOpt := opt.apply(svrOpts); grpcOpt != nil {
			grpcOpts = append(grpcOpts, grpcOpt)
		}
	}

	m, err := NewMux()
	if err != nil {
		return nil, err
	}
	m.opts = svrOpts.muxOpts
	grpcOpts = append(grpcOpts, grpc.UnknownServiceHandler(m.StreamHandler()))

	return &Server{
		opts:     svrOpts,
		grpcOpts: grpcOpts,
		mux:      m,
		closer:   make(chan struct{}),
	}, nil
}

func (s *Server) RegisterConn(cc *grpc.ClientConn) {
	// TODO...
}

func (s *Server) Serve(l net.Listener) error {
	m := cmux.New(l)

	grpcL := m.Match(cmux.HTTP2HeaderField("content-type", "application/grpc"))
	httpL := m.Match(cmux.Any())

	n := 3
	errs := make(chan error, n)

	gs := grpc.NewServer(s.grpcOpts...)
	go func() { errs <- gs.Serve(grpcL) }()
	defer gs.Stop()

	hs := &http.Server{
		Handler:   s.mux,
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

const (
	defaultServerMaxReceiveMessageSize = 1024 * 1024 * 4
	defaultServerMaxSendMessageSize    = math.MaxInt32
	defaultServerConnectionTimeout     = 120 * time.Second
)

type serverOptions struct {
	tlsConfig *tls.Config
	muxOpts   *muxOptions
}

var defaultServerOptions = serverOptions{
	muxOpts: &defaultMuxOptions,
}

type ServerOption interface {
	apply(*serverOptions) grpc.ServerOption
}

type serverOptionFunc func(s *serverOptions) grpc.ServerOption

func (s serverOptionFunc) apply(opts *serverOptions) grpc.ServerOption { return s(opts) }

func TLSCreds(c *tls.Config) ServerOption {
	return serverOptionFunc(func(opts *serverOptions) grpc.ServerOption {
		opts.tlsConfig = c
		creds := credentials.NewTLS(c)
		return grpc.Creds(creds)
	})
}
