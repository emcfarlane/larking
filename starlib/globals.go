// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlib

import (
	"larking.io/starlib/starlarkstruct"
	"go.starlark.net/resolve"
	"go.starlark.net/starlark"
)

func init() {
	resolve.AllowSet = true
	resolve.AllowGlobalReassign = true
	resolve.AllowRecursion = true

	// TODO: requirement for REPL.
	resolve.LoadBindsGlobally = true
}

func NewGlobals() starlark.StringDict {
	return starlark.StringDict{
		"struct": starlark.NewBuiltin("struct", starlarkstruct.Make),
		"module": starlark.NewBuiltin("module", starlarkstruct.MakeModule),
	}
}
