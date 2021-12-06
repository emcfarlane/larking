package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/emcfarlane/larking"
	"github.com/go-logr/logr"
	"github.com/go-logr/stdr"
	"google.golang.org/grpc"
)

const defaultAddr = "localhost:6060" // default webserver address

// TODO: config, for now flags!
var (
	httpAddr string
	services stringFlags
)

type stringFlags []string

func (i *stringFlags) String() string { return fmt.Sprintf("%#v", i) }
func (i *stringFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}

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

	l, err := net.Listen("tcp", httpAddr)
	if err != nil {
		return err
	}
	defer l.Close()

	stdr.SetVerbosity(1)
	log := stdr.NewWithOptions(log.New(os.Stderr, "", log.LstdFlags), stdr.Options{LogCaller: stdr.All})
	log = log.WithName("Larking")

	unary := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		ctx = logr.NewContext(ctx, log)
		return handler(ctx, req)
	}

	stream := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		return handler(srv, logStream{
			ServerStream: ss,
			log:          log,
		})
	}

	mux, err := larking.NewMux(
		larking.UnaryServerInterceptorOption(unary),
		larking.StreamServerInterceptorOption(stream),
	)
	if err != nil {
		return err
	}

	s, err := larking.NewServer(mux,
		larking.LarkingServerOption(
			map[string]string{
				"default": "", // empty default
			},
		),
	)
	if err != nil {
		return err
	}

	// TODO:
	for _, svc := range services {
		cc, err := grpc.DialContext(ctx, svc, grpc.WithInsecure())
		if err != nil {
			return err
		}

		if err := mux.RegisterConn(ctx, cc); err != nil {
			return err
		}
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
	flag.StringVar(&httpAddr, "addr", defaultAddr, "HTTP service address")
	flag.Var(&services, "svc", "GRPC service to proxy")
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
