package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net"

	"github.com/emcfarlane/larking"
	"github.com/emcfarlane/larking/examples/library/apipb"
	_ "modernc.org/sqlite"
)

// Server implement LibraryServer.
// TODO: add all implemented methods to this struct.
type Server struct {
	apipb.UnimplementedLibraryServer

	db *sql.DB
}

var (
	flagPort = flag.String("port", "8000", "Port")
)

func run() error {
	flag.Parse()

	db, err := sql.Open("sqlite", "sqlite:file::memory:?cache=shared")
	if err != nil {
		return err
	}
	defer db.Close()
	if err := createTables(db); err != nil {
		return err
	}

	s := &Server{db: db}

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
