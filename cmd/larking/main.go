package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/emcfarlane/larking"
	"github.com/emcfarlane/larking/api/healthpb"
	"github.com/emcfarlane/larking/api/workerpb"
	"github.com/emcfarlane/larking/health"
	"github.com/emcfarlane/larking/starlib"
	"github.com/emcfarlane/larking/worker"
	"github.com/go-logr/logr"
	"github.com/go-logr/stdr"
	"google.golang.org/grpc"
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
	flagMain        = flag.String("main", "", "Main thread for worker.")
	flagInsecure    = flag.Bool("insecure", false, "Insecure, disabled credentials.")
)

type logStream struct {
	grpc.ServerStream
	log logr.Logger
}

func (s logStream) Context() context.Context {
	return logr.NewContext(s.ServerStream.Context(), s.log)
}

func run(ctx context.Context) error {
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
		log.Info("unary", info)
		ctx = logr.NewContext(ctx, log)
		defer log.Info("unary end", info)
		return handler(ctx, req)
	}

	stream := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		log.Info("stream", info)
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

	workerServer := worker.NewServer()
	workerServer.Load = starlib.StdLoad
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

	//// TODO:
	//for _, svc := range services {
	//	cc, err := grpc.DialContext(ctx, svc, grpc.WithInsecure())
	//	if err != nil {
	//		return err
	//	}
	//	if err := mux.RegisterConn(ctx, cc); err != nil {
	//		return err
	//	}
	//}

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
