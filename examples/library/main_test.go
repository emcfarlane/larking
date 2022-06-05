package main

import (
	"database/sql"
	"testing"

	"github.com/emcfarlane/larking"
	"github.com/emcfarlane/larking/examples/library/apipb"
	"github.com/emcfarlane/larking/starlib"
	"go.starlark.net/starlark"
)

func TestScripts(t *testing.T) {
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := createTables(db); err != nil {
		t.Fatal(err)
	}

	s := &Server{db: db}

	mux, err := larking.NewMux()
	if err != nil {
		t.Fatal(err)
	}
	mux.RegisterService(&apipb.Library_ServiceDesc, s)

	starlib.RunTests(t, "testdata/*.star", starlark.StringDict{
		"mux": mux,
	})
}
