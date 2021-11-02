// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlarkerrors

import (
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/starlarktest"
)

func load(thread *starlark.Thread, module string) (starlark.StringDict, error) {
	if module == "assert.star" {
		return starlarktest.LoadAssertModule()
	}
	return nil, fmt.Errorf("unknown module %s", module)
}

//

// ioEOF fails with an io.EOF error.
func ioEOF(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return nil, io.EOF
}

func TestExecFile(t *testing.T) {
	thread := &starlark.Thread{Load: load}
	starlarktest.SetReporter(thread, t)
	globals := starlark.StringDict{
		"struct":         starlark.NewBuiltin("struct", starlarkstruct.Make),
		"errors":         NewModule(),
		"io_eof":         NewError(io.EOF),
		"io_eof_wrapped": NewError(fmt.Errorf("wrapped: %w", io.EOF)),
		"io_eof_func":    starlark.NewBuiltin("io_eof_func", ioEOF),
	}

	files, err := filepath.Glob("testdata/*.star")
	if err != nil {
		t.Fatal(err)
	}

	for _, filename := range files {
		src, err := ioutil.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}

		_, err = starlark.ExecFile(thread, filename, src, globals)
		switch err := err.(type) {
		case *starlark.EvalError:
			var found bool
			for i := range err.CallStack {
				posn := err.CallStack.At(i).Pos
				if posn.Filename() == filename {
					linenum := int(posn.Line)
					msg := err.Error()

					t.Errorf("\n%s:%d: unexpected error: %v", filename, linenum, msg)
					found = true
					break
				}
			}
			if !found {
				t.Error(err.Backtrace())
			}
		case nil:
			// success
		default:
			t.Errorf("\n%s", err)
		}

	}
}
