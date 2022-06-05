// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlarkruntimevar_test

import (
	"testing"

	"github.com/emcfarlane/larking/starlib"
	"go.starlark.net/starlark"

	_ "gocloud.dev/runtimevar/constantvar"
)

func TestExecFile(t *testing.T) {
	globals := starlark.StringDict{}
	starlib.RunTests(t, "testdata/*_test.star", globals)
}
