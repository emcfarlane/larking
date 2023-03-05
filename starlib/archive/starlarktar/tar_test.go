// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlarktar_test

import (
	"testing"

	"larking.io/starlib"

	_ "modernc.org/sqlite"
)

func TestExecFile(t *testing.T) {
	starlib.RunTests(t, "testdata/*.star", nil)
}
