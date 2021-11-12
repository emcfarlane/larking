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

	"github.com/emcfarlane/larking/api"
	"github.com/emcfarlane/larking/starlarkthread"
	"github.com/soheilhy/cmux"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
	"golang.org/x/net/trace"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

type Server struct {
	opts serverOptions
	mux  *Mux

	closer chan bool
	gs     *grpc.Server
	hs     *http.Server

	events trace.EventLog

	api.UnimplementedLarkingServer
	threads map[string]*Thread // starlark.StringDict
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
	// TODO: auth.
	s.RegisterService(&api.Larking_ServiceDesc, s)

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

type Thread struct {
	name      string
	globals   starlark.StringDict
	createdAt time.Time
}

func NewThreadAt(name string, globals starlark.StringDict, at time.Time) *Thread {
	for _, val := range globals {
		val.Freeze() // freeze each value
	}
	return &Thread{
		name:      name,
		globals:   globals,
		createdAt: at,
	}
}

func (t *Thread) Predeclared() starlark.StringDict {
	predeclared := make(starlark.StringDict, len(t.globals))
	for key, val := range t.globals {
		predeclared[key] = val
	}
	return predeclared
}

func soleExpr(f *syntax.File) syntax.Expr {
	if len(f.Stmts) == 1 {
		if stmt, ok := f.Stmts[0].(*syntax.ExprStmt); ok {
			return stmt.X
		}
	}
	return nil
}

// errorStatus creates a status from an error,
// or its backtrace if it is a Starlark evaluation error.
func errorStatus(err error) *status.Status {
	st := status.New(codes.InvalidArgument, err.Error())
	if evalErr, ok := err.(*starlark.EvalError); ok {
		// Describes additional debugging info.
		di := &errdetails.DebugInfo{
			StackEntries: strings.Split(evalErr.Backtrace(), "\n"),
			Detail:       "<repl>",
		}
		st, err = st.WithDetails(di)
		if err != nil {
			// If this errored, it will always error
			// here, so better panic so we can figure
			// out why than have this silently passing.
			panic(fmt.Sprintf("Unexpected error: %v", err))
		}
	}
	return st

}

// Create ServerStream...
func (s *Server) RunOnThread(stream api.Larking_RunOnThreadServer) error {
	ctx := stream.Context()

	cmd, err := stream.Recv()
	if err != nil {
		return err
	}

	serverThread, ok := s.threads[cmd.Name]
	if !ok {
		return status.Error(codes.NotFound, "unknown thread")
	}

	thread := new(starlark.Thread) // TODO: thread creation.

	starlarkthread.SetContext(thread, ctx)
	cleanup := starlarkthread.WithResourceStore(thread)
	defer func() {
		if err := cleanup(); err != nil {
			fmt.Println("err:", err)
		}
	}()

	// globals need to be copied...
	globals := serverThread.Predeclared()

	for {
		f, err := syntax.Parse(serverThread.name, cmd.Input, 0)
		//f, err := syntax.ParseCompoundStmt("<stdin>", readline)
		if err != nil {
			return errorStatus(err).Err()
		}

		var output string
		if expr := soleExpr(f); expr != nil {
			// eval
			v, err := starlark.EvalExpr(thread, expr, globals)
			if err != nil {
				return errorStatus(err).Err()
			}

			// print
			if v != starlark.None {
				output = v.String()
			}
		} else if err := starlark.ExecREPLChunk(f, thread, globals); err != nil {
			return errorStatus(err).Err()
		} else {
			output = ""
		}

		result := &api.Result{
			Output: output,
		}

		if err = stream.Send(result); err != nil {
			return err
		}

		cmd, err = stream.Recv()
		if err != nil {
			return err
		}
	}
}

func (s *Server) ListThreads(context.Context, *api.ListThreadsRequest) (*api.ListThreadsResponse, error) {
	return nil, nil
}
