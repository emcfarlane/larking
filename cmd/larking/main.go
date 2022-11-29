package main

import (
	"context"
	"crypto/x509"
	"flag"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/go-logr/logr"
	"github.com/go-logr/stdr"
	"go.starlark.net/starlark"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"larking.io"
	"larking.io/api/controlpb"
	"larking.io/api/healthpb"
	"larking.io/api/workerpb"
	_ "larking.io/cmd/internal/bindings"
	"larking.io/control"
	"larking.io/health"
	"larking.io/starlib"
	"larking.io/starlib/starlarkthread"
	"larking.io/worker"
)

const defaultAddr = "localhost:6060" // default webserver address

func env(key, def string) string {
	if e := os.Getenv(key); e != "" {
		return e
	}
	return def
}

var (
	flagAddr        = flag.String("addr", env("LARKING_ADDRESS", defaultAddr), "Local address to listen on.")
	flagControlAddr = flag.String("control", "https://larking.io", "Control server for credentials.")
	flagInsecure    = flag.Bool("insecure", false, "Insecure, disabled credentials.")
	flagCreds       = flag.String("credentials", env("WORKER_CREDENTIALS", ""), "Runtime variable for credentials.")

	flagDir = flag.String("dir", env("LARKING_DIR", "file://./?metadata=skip"), "Set the module loading directory")
)

type logStream struct {
	grpc.ServerStream
	log logr.Logger
}

func (s logStream) Context() context.Context {
	return logr.NewContext(s.ServerStream.Context(), s.log)
}

func run(ctx context.Context) (err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	l, err := net.Listen("tcp", *flagAddr)
	if err != nil {
		return err
	}
	defer l.Close()

	stdr.SetVerbosity(1)
	log := stdr.NewWithOptions(log.New(os.Stderr, "", log.LstdFlags), stdr.Options{LogCaller: stdr.All})
	log = log.WithName("Larking")

	unary := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		log.Info("unary request", "info", info)
		ctx = logr.NewContext(ctx, log)
		defer log.Info("unary end", info)
		return handler(ctx, req)
	}

	stream := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		log.Info("stream request", "info", info)
		defer log.Info("stream end", info)
		return handler(srv, logStream{
			ServerStream: ss,
			log:          log,
		})
	}

	var muxOpts = []larking.MuxOption{
		larking.UnaryServerInterceptorOption(unary),
		larking.StreamServerInterceptorOption(stream),
	}

	mux, err := larking.NewMux(muxOpts...)
	if err != nil {
		return err
	}

	healthServer := health.NewServer()
	defer healthServer.Shutdown()
	mux.RegisterService(&healthpb.Health_ServiceDesc, healthServer)

	globals := starlib.NewGlobals()
	loader := starlib.NewLoader(globals)
	defer loader.Close()

	var (
		controlClient controlpb.ControlClient = control.InsecureControlClient{}
		name          string
	)
	if !*flagInsecure || *flagCreds != "" {
		log.Info("loading worker credentials")

		perRPC, err := control.OpenRPCCredentials(ctx, *flagCreds)
		if err != nil {
			return err
		}
		defer perRPC.Close()

		name = perRPC.Name()

		// TODO: load creds.
		pool, err := x509.SystemCertPool()
		if err != nil {
			return err
		}
		creds := credentials.NewClientTLSFromCert(pool, "")

		cc, err := grpc.DialContext(
			ctx, *flagControlAddr,
			grpc.WithTransportCredentials(creds),
			grpc.WithPerRPCCredentials(perRPC),
		)
		if err != nil {
			return err
		}
		defer cc.Close()

		controlClient = controlpb.NewControlClient(cc)
	}

	workerServer := worker.NewServer(
		loader.Load,
		controlClient,
		name,
	)
	mux.RegisterService(&workerpb.Worker_ServiceDesc, workerServer)
	healthServer.SetServingStatus(
		workerpb.Worker_ServiceDesc.ServiceName,
		healthpb.HealthCheckResponse_SERVING,
	)

	var svrOpts []larking.ServerOption

	if *flagInsecure {
		svrOpts = append(svrOpts, larking.InsecureServerOption())
	}

	s, err := larking.NewServer(mux, svrOpts...)
	if err != nil {
		return err
	}

	// Run script
	switch flag.NArg() {
	case 0:
		// nothing
	case 1:
		name := flag.Arg(0)

		src, err := loader.LoadSource(ctx, *flagDir, name)
		if err != nil {
			return err
		}

		globals := starlib.NewGlobals()
		globals["mux"] = mux // add mux!
		thread := &starlark.Thread{
			Name: name, // TODO: name encoding...
			Load: loader.Load,
			Print: func(_ *starlark.Thread, msg string) {
				log.Info(msg, "thread", name)
			},
		}
		starlarkthread.SetContext(thread, ctx)
		close := starlarkthread.WithResourceStore(thread)
		defer func() {
			if cerr := close(); err == nil {
				err = cerr
			}
		}()

		module, err := starlark.ExecFile(thread, name, src, globals)
		if err != nil {
			return err
		}
		if mainFn, ok := module["main"]; ok {
			if _, err := starlark.Call(thread, mainFn, nil, nil); err != nil {
				return err
			}
		}

	default:
		return fmt.Errorf("unexpected number of args")
	}

	go func() {
		log.Info("listening", "address", l.Addr().String())
		if err := s.Serve(l); err != nil {
			log.Error(err, "server stopped")
		}
		cancel()
	}()
	<-ctx.Done()
	return s.Shutdown(ctx)
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage: larking -addr="+defaultAddr+" -svc=service1 -svc=service2\n")
	flag.PrintDefaults()
	os.Exit(2)
}

func main() {
	flag.Usage = usage
	flag.Parse()

	//if len(services) == 0 {
	//	usage()
	//}

	ctx := context.Background()
	ctx, cancel := larking.NewOSSignalContext(ctx)
	defer cancel()

	if err := run(ctx); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
