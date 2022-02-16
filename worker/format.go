// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package worker

import (
	"context"

	"github.com/bazelbuild/buildtools/build"
	"github.com/bazelbuild/buildtools/convertast"
	"go.starlark.net/syntax"
)

// Format starlark code.
func Format(_ context.Context, filename string, src interface{}) ([]byte, error) {
	ast, err := syntax.Parse(filename, src, syntax.RetainComments)
	if err != nil {
		return nil, err
	}
	newAst := convertast.ConvFile(ast)
	return build.Format(newAst), nil
}
