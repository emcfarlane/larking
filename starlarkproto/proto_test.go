package starlarkproto

import (
	"testing"

	"github.com/emcfarlane/larking/starlarkstruct"
	"github.com/emcfarlane/larking/starlarkthread"
	"github.com/emcfarlane/starlarkassert"
	"go.starlark.net/starlark"

	_ "github.com/emcfarlane/starlarkproto/testpb"
)

func TestProto(t *testing.T) {
	globals := starlark.StringDict{
		"struct": starlark.NewBuiltin("struct", starlarkstruct.Make),
		"proto":  NewModule(),
	}
	starlarkassert.RunTests(t, "testdata/*.star", globals, starlarkthread.AssertOption)
}
