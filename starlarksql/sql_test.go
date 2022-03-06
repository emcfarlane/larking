// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlarksql

import (
	"testing"

	"github.com/emcfarlane/larking/starlarkstruct"
	"github.com/emcfarlane/larking/starlarkthread"
	"github.com/emcfarlane/starlarkassert"
	"go.starlark.net/starlark"

	_ "modernc.org/sqlite"
)

func TestExecFile(t *testing.T) {
	globals := starlark.StringDict{
		"struct": starlark.NewBuiltin("struct", starlarkstruct.Make),
		"sql":    NewModule(),
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
