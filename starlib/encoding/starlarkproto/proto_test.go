package starlarkproto_test

import (
	"testing"

	_ "github.com/emcfarlane/starlarkproto/testpb"
	"go.starlark.net/starlark"
	"larking.io/starlib"
)

func TestProto(t *testing.T) {
	globals := starlark.StringDict{
		// TODO
		//"library_bin": starlark.String(b),
	}
	starlib.RunTests(t, "testdata/*.star", globals)
}
