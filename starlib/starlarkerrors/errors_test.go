// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlarkerrors_test

import (
	"io"
	"testing"

	"larking.io/starlib"
	"larking.io/starlib/starlarkerrors"
	"go.starlark.net/starlark"
)

// ioEOF fails with an io.EOF error.
func ioEOF(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return nil, io.EOF
}

func TestExecFile(t *testing.T) {
	globals := starlark.StringDict{
		"io_eof":      starlarkerrors.NewError(io.EOF),
		"io_eof_func": starlark.NewBuiltin("io_eof_func", ioEOF),
	}
	starlib.RunTests(t, "testdata/*.star", globals)
}
