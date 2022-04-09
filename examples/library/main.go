package main

import (
	"flag"
	"fmt"
	"log"
	"net"

	"github.com/emcfarlane/larking"
	"github.com/emcfarlane/larking/examples/library/apipb"
)

// Server implement LibraryServer.
// TODO: add all implemented methods to this struct.
type Server struct {
	apipb.UnimplementedLibraryServer
}

var (
	flagPort = flag.String("port", "8000", "Port")
)

func run() error {
	flag.Parse()
	s := &Server{}

	mux, err := larking.NewMux()
	if err != nil {
		return err
	}
	mux.RegisterService(&apipb.Library_ServiceDesc, s)

	svr, err := larking.NewServer(mux,
		larking.InsecureServerOption(),
	)
	if err != nil {
		return err
	}

	l, err := net.Listen("tcp", fmt.Sprintf(":%s", *flagPort))
	if err != nil {
		return err
	}
	log.Printf("listening on %s", l.Addr().String())
	return svr.Serve(l)
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
