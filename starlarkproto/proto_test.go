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
	runner := func(t testing.TB, thread *starlark.Thread) func() {
		close := starlarkthread.WithResourceStore(thread)
		return func() {
			if err := close(); err != nil {
				t.Error(err, "failed to close resources")
			}
		}
	}
	starlarkassert.RunTests(t, "testdata/*.star", globals, runner)

}
