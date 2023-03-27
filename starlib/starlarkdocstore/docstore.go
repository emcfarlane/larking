// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package starlarkdocstore supports noSQL document databases.
package starlarkdocstore

import (
	"fmt"

	"go.starlark.net/starlark"
	"larking.io/starlib/starlarkstruct"
)

func NewModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "docstore",
		Members: starlark.StringDict{
			"open": starlark.NewBuiltin("docstore.open", Open),
		},
	}
}

func Open(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return nil, fmt.Errorf("TODO")
}
