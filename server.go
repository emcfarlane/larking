// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package larking

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
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
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
	"golang.org/x/net/http2"
	"golang.org/x/net/trace"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/emcfarlane/larking/api"
	"github.com/emcfarlane/larking/starlarkthread"
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
	ls     *LarkingServer

	events trace.EventLog
}

// NewServer creates a new Proxy server.
func NewServer(mux *Mux, opts ...ServerOption) (*Server, error) {
	if mux == nil {
		return nil, fmt.Errorf("invalid mux must not be nil")
	}

	var svrOpts serverOptions
	for _, opt := range opts {
		opt(&svrOpts)
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
	//if config := svrOpts.tlsConfig; config != nil {
	//	creds = credentials.NewTLS(config)
	//}
	grpcOpts = append(grpcOpts, grpc.Creds(creds))

	var ls *LarkingServer
	if svrOpts.larkingEnabled {
		loader, err := NewLoader()
		if err != nil {
			return nil, err
		}

		threads := make(map[string]*Thread)
		for name, src := range svrOpts.larkingThreads {

			var buf bytes.Buffer
			thread := &starlark.Thread{
				Name: name,
				Print: func(_ *starlark.Thread, msg string) {
					buf.WriteString(msg) //nolint
				},
				Load: loader.Load,
			}
			globals, err := starlark.ExecFile(thread, name, src, nil)
			if err != nil {
				return nil, err
			}
			threads[name] = &Thread{
				name:    name,
				globals: globals,
			}
		}
		ls = &LarkingServer{
			threads: threads,
		}
	}

	gs := grpc.NewServer(grpcOpts...)
	hs := &http.Server{
		Handler: svrOpts.serveMux,
		//TLSConfig: svrOpts.tlsConfig,
	}

	// Register local gRPC services
	for sd, ss := range mux.services {
		gs.RegisterService(sd, ss)
	}

	return &Server{
		opts:   svrOpts,
		mux:    mux,
		gs:     gs,
		hs:     hs,
		ls:     ls,
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

	// TODO: metrics/debug http server...?
	// TODO: auth.
	if s.ls != nil {
		s.gs.RegisterService(&api.Larking_ServiceDesc, s.ls)
		s.mux.RegisterService(&api.Larking_ServiceDesc, s.ls)
	}
	//if s.healthServer != nil {
	//	s.healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	//	s.RegisterService(&healthpb.Health_ServiceDesc, s.healthServer)
	//}

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
	tlsConfig      *tls.Config
	larkingEnabled bool
	larkingThreads map[string]string // name->src  TODO: something better
	log            logr.Logger

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

func LarkingServerOption(threads map[string]string) ServerOption {
	return func(opts *serverOptions) error {
		opts.larkingEnabled = true
		opts.larkingThreads = threads
		return nil
	}
}

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

type Thread struct {
	name      string
	globals   starlark.StringDict
	loader    *Loader
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

type statusError interface{ GRPCStatus() *status.Status }

func FprintErr(w io.Writer, err error) {
	switch v := err.(type) {
	case *starlark.EvalError:
		fmt.Fprintln(w, v.Backtrace())
	case statusError:
		s := v.GRPCStatus()
		p := s.Proto()
		for _, detail := range p.Details {
			m, err := detail.UnmarshalNew()
			if err != nil {
				fmt.Fprintf(w, "InternalError: %v\n", err)
				continue
			}
			switch m := m.(type) {
			case *errdetails.DebugInfo:
				for _, se := range m.StackEntries {
					fmt.Fprintln(w, se)
				}
			default:
				fmt.Fprintf(w, "%v\n", m)
			}
		}
		if len(p.Details) == 0 {
			fmt.Fprintf(w, "Error: %v: %s\n", s.Code(), s.Message())
		}
	default:
		fmt.Fprintln(w, err)
	}
}

// ErrorStatus creates a status from an error,
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

type funcReader func(p []byte) (n int, err error)

func (f funcReader) Read(p []byte) (n int, err error) { return f(p) }

type LarkingServer struct {
	api.UnimplementedLarkingServer

	threads map[string]*Thread // starlark.StringDict
}

// Create ServerStream...
func (s *LarkingServer) RunOnThread(stream api.Larking_RunOnThreadServer) error {
	ctx := stream.Context()
	l := logr.FromContextOrDiscard(ctx)

	cmd, err := stream.Recv()
	if err != nil {
		return err
	}
	l.Info("running on thread", "thread", cmd.Name)

	serverThread, ok := s.threads[cmd.Name]
	if !ok {
		return status.Error(codes.NotFound, "unknown thread")
	}

	// TODO: server loader
	loader, err := NewLoader()
	if err != nil {
		return err
	}

	// TODO: better thread creation.
	var buf bytes.Buffer
	thread := &starlark.Thread{
		Name: serverThread.name,
		Print: func(_ *starlark.Thread, msg string) {
			buf.WriteString(msg) //nolint
		},
		Load: loader.Load,
	}

	starlarkthread.SetContext(thread, ctx)
	cleanup := starlarkthread.WithResourceStore(thread)
	defer func() {
		if err := cleanup(); err != nil {
			// TODO: log
			fmt.Println("err:", err)
		}
	}()

	// globals need to be copied...
	globals := serverThread.Predeclared()

	c := Completer{globals}

	// TODO: buffer by lines?
	//var buf bytes.Buffer
	//if _, err := buf.WriteString(cmd.Input); err != nil {
	//	return err
	//}
	//reader := func(p []byte) (n int, err error) {
	//	for buf.Len() == 0 {
	//		cmd, err := stream.Recv()
	//		if err != nil {
	//			// err == io.EOF?
	//			return 0, err
	//		}
	//		if _, err := buf.WriteString(cmd.Input); err != nil {
	//			return 0, err
	//		}
	//	}
	//	return buf.Read(p)
	//}
	//scanner := bufio.NewScanner(funcReader(reader))
	//readline := func() ([]byte, error) {
	//	for scanner.Scan() {
	//		return scanner.Bytes(), nil
	//	}
	//	if err := scanner.Err(); err != nil {
	//		return nil, err
	//	}
	//	return nil, io.EOF
	//}

	for {
		switch v := cmd.Exec.(type) {
		case *api.Command_Input:
			buf.Reset()

			f, err := syntax.Parse(serverThread.name, v.Input, 0)
			//f, err := syntax.ParseCompoundStmt(fmt.Sprintf("<%s>", serverThread.name), readline)
			if err != nil {
				return errorStatus(err).Err()
			}

			if expr := soleExpr(f); expr != nil {
				// eval
				v, err := starlark.EvalExpr(thread, expr, globals)
				if err != nil {
					return errorStatus(err).Err()
				}

				// print
				if v != starlark.None {
					buf.WriteString(v.String())
				}
			} else if err := starlark.ExecREPLChunk(f, thread, globals); err != nil {
				return errorStatus(err).Err()
			}

			result := &api.Result{
				Result: &api.Result_Output{
					Output: &api.Output{
						//Input:  v.Input,
						Output: buf.String(),
					},
				},
			}

			if err = stream.Send(result); err != nil {
				return err
			}

		case *api.Command_Complete:
			completions := c.Complete(v.Complete)

			result := &api.Result{
				Result: &api.Result_Completion{
					Completion: &api.Completion{
						Completions: completions,
					},
				},
			}

			if err = stream.Send(result); err != nil {
				return err
			}
		}

		cmd, err = stream.Recv()
		if err != nil {
			return err
		}
	}
}

func (s *LarkingServer) ListThreads(context.Context, *api.ListThreadsRequest) (*api.ListThreadsResponse, error) {
	var threads []*api.Thread
	for _, v := range s.threads {
		threads = append(threads, &api.Thread{
			Name: v.name,
		})
	}
	return &api.ListThreadsResponse{
		Threads: threads,
	}, nil
}
