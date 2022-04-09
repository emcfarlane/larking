package main

import (
	"testing"

	"github.com/emcfarlane/larking"
	"github.com/emcfarlane/larking/examples/library/apipb"
	"github.com/emcfarlane/larking/starlib"
	"go.starlark.net/starlark"
)

func TestScripts(t *testing.T) {
	s := &Server{}

	mux, err := larking.NewMux()
	if err != nil {
		t.Fatal(err)
	}
	mux.RegisterService(&apipb.Library_ServiceDesc, s)

	starlib.RunTests(t, "testdata/*.star", starlark.StringDict{
		"mux": mux,
	})
}
