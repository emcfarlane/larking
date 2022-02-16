// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlarkerrors

import (
	"io"
	"testing"

	"github.com/emcfarlane/larking/starlarkthread"
	"github.com/emcfarlane/starlarkassert"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// ioEOF fails with an io.EOF error.
func ioEOF(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return nil, io.EOF
}

func TestExecFile(t *testing.T) {
	globals := starlark.StringDict{
		"struct":      starlark.NewBuiltin("struct", starlarkstruct.Make),
		"errors":      NewModule(),
		"io_eof":      NewError(io.EOF),
		"io_eof_func": starlark.NewBuiltin("io_eof_func", ioEOF),
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
