// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlarkblob

import (
	"testing"

	"github.com/emcfarlane/larking/starlarkthread"
	"github.com/emcfarlane/starlarkassert"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	_ "gocloud.dev/blob/memblob"
)

func TestExecFile(t *testing.T) {
	runner := func(thread *starlark.Thread, handler func() error) (err error) {
		close := starlarkthread.WithResourceStore(thread)
		defer func() {
			cerr := close()
			if err == nil {
				err = cerr
			}
		}()
		return handler()
	}
	globals := starlark.StringDict{
		"struct": starlark.NewBuiltin("struct", starlarkstruct.Make),
		"blob":   NewModule(),
	}
	starlarkassert.RunTests(t, "testdata/*.star", globals, runner)
}
