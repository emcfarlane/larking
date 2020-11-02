package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/emcfarlane/graphpb"
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

func run() error {
	l, err := net.Listen("tcp", httpAddr)
	if err != nil {
		return err
	}
	defer l.Close()

	ctx := context.TODO()
	s, err := graphpb.NewServer(ctx, []string(services))
	if err != nil {
		return err
	}

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		s.Close()
	}()

	return s.Serve(l)
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage: graphpb -addr="+defaultAddr+" -svc=service1 -svc=service2\n")
	flag.PrintDefaults()
	os.Exit(2)
}

func main() {
	flag.Usage = usage
	flag.StringVar(&httpAddr, "addr", defaultAddr, "HTTP service address")
	flag.Var(&services, "svc", "GRPC service to proxy")
	flag.Parse()

	if len(services) == 0 {
		usage()
	}

	if err := run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
