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
	runner := func(thread *starlark.Thread, handler func() error) (err error) {
		close := starlarkthread.WithResourceStore(thread)
		defer func() {
			if cerr := close(); err == nil {
				err = cerr
			}
		}()
		return handler()
	}
	starlarkassert.RunTests(t, "testdata/*.star", globals, runner)

}
