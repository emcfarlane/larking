package starlarkproto

import (
	"io/ioutil"
	"testing"

	"github.com/emcfarlane/larking/starlarkstruct"
	"github.com/emcfarlane/larking/starlarkthread"
	"github.com/emcfarlane/starlarkassert"
	"go.starlark.net/starlark"

	_ "github.com/emcfarlane/starlarkproto/testpb"
)

func TestProto(t *testing.T) {
	b, err := ioutil.ReadFile("testdata/library.bin")
	if err != nil {
		t.Fatal(err)
	}

	globals := starlark.StringDict{
		"struct":      starlark.NewBuiltin("struct", starlarkstruct.Make),
		"proto":       NewModule(),
		"library_bin": starlark.String(b),
	}
	starlarkassert.RunTests(t, "testdata/*.star", globals, starlarkthread.AssertOption)
}
