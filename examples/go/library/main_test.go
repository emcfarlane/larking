package main

import (
	"testing"

	"github.com/emcfarlane/larking"
	"github.com/emcfarlane/larking/examples/go/library/apipb"
	"github.com/emcfarlane/larking/starlib"
	"github.com/emcfarlane/starlarkassert"
	"go.starlark.net/starlark"
)

func TestScripts(t *testing.T) {
	s := &Server{}

	mux, err := larking.NewMux()
	if err != nil {
		t.Fatal(err)
	}
	mux.RegisterService(&apipb.Library_ServiceDesc, s)

	globals := starlark.StringDict{
		"mux": mux,
	}
	loadOpt := starlarkassert.WithLoad(starlib.StdLoad)
	starlarkassert.RunTests(t, "testdata/*.star", globals, loadOpt)
}
