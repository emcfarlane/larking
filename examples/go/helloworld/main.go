package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/emcfarlane/graphpb"
	pb "github.com/emcfarlane/graphpb/examples/proto/helloworld"
	"github.com/emcfarlane/graphpb/grpc/reflection"
	"github.com/soheilhy/cmux"
	"google.golang.org/grpc"
)

var (
	flagPort = flag.String("port", "8000", "Port")
	flagHost = flag.String("host", "", "Host")
)

type Server struct {
	pb.UnimplementedGreeterServer
}

func (s *Server) serve(l net.Listener) error {
	m := cmux.New(l)

	grpcL := m.Match(cmux.HTTP2HeaderField("content-type", "application/grpc"))
	httpL := m.Match(cmux.Any())

	gs := grpc.NewServer()
	pb.RegisterGreeterServer(gs, s)
	reflection.Register(gs)

	// Register HTTP handler in-process to server both the GRPC server and
	// the HTTP server on one port.
	hd := &graphpb.Handler{}
	if err := hd.RegisterServiceByName("helloworld.Greeter", s); err != nil {
		return err
	}

	hs := &http.Server{
		Handler: hd,
	}

	errs := make(chan error, 3)

	go func() { errs <- gs.Serve(grpcL) }()
	defer gs.Stop()

	go func() { errs <- hs.Serve(httpL) }()
	defer hs.Close()

	go func() { errs <- m.Serve() }()

	return <-errs
}

func run() error {
	flag.Parse()

	l, err := net.Listen("tcp", fmt.Sprintf("%s:%s", *flagHost, *flagPort))
	if err != nil {
		return err
	}

	s := &Server{}
	return s.serve(l)
}

func main() {
	flag.Parse()

	if err := run(); err != nil {
		log.Fatal(err)
	}
}
