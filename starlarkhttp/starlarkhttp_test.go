// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlarkhttp

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/emcfarlane/larking/starlarkthread"
	"github.com/emcfarlane/starlarkassert"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

func TestExecFile(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/hello", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "world")
	})

	// Create a test http server.
	ts := httptest.NewServer(mux)
	defer ts.Close()

	globals := starlark.StringDict{
		"struct": starlark.NewBuiltin("struct", starlarkstruct.Make),
		"addr":   starlark.String(ts.URL),
		"http":   NewModule(),
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
