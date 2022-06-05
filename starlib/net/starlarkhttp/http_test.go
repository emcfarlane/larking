// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlarkhttp_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"larking.io/starlib"
	"go.starlark.net/starlark"
)

func TestExecFile(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/hello", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "world")
	})

	// Create a test http server.
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	globals := starlark.StringDict{
		"addr": starlark.String(ts.URL),
	}
	starlib.RunTests(t, "testdata/*.star", globals)
}
