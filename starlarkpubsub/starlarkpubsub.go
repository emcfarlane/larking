// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package starlarkpubsub provides methods for publishing and recieving messages.
package starlarkpubsub

import (
	"fmt"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

func NewModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "pubsub",
		Members: starlark.StringDict{
			"open": starlark.NewBuiltin("pubsub.open", Open),
		},
	}
}

func Open(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return nil, fmt.Errorf("TODO")
}
