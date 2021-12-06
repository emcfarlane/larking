// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlarkruntimevar

import (
	"testing"

	"github.com/emcfarlane/larking/starlarkthread"
	"github.com/emcfarlane/starlarkassert"
	starlarktime "go.starlark.net/lib/time"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	_ "gocloud.dev/runtimevar/constantvar"
)

func TestExecFile(t *testing.T) {
	globals := starlark.StringDict{
		"struct":     starlark.NewBuiltin("struct", starlarkstruct.Make),
		"runtimevar": NewModule(),
		"time":       starlarktime.Module,
	}
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
	starlarkassert.RunTests(t, "testdata/*.star", globals, runner)
}
