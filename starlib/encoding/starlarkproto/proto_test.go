package starlarkproto_test

import (
	"testing"

	"github.com/emcfarlane/larking/starlib"
	_ "github.com/emcfarlane/starlarkproto/testpb"
	"go.starlark.net/starlark"
)

func TestProto(t *testing.T) {
	globals := starlark.StringDict{
		// TODO
		//"library_bin": starlark.String(b),
	}
	starlib.RunTests(t, "testdata/*_test.star", globals)
}
