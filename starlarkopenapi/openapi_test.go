// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlarkopenapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/emcfarlane/larking/starlarkthread"
	"github.com/emcfarlane/starlarkassert"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	_ "gocloud.dev/runtimevar/filevar"
)

func TestExecFile(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/hello", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "world")
	})

	// Create a test http server.
	ts := httptest.NewServer(mux)
	defer ts.Close()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Log(wd)

	globals := starlark.StringDict{
		"struct": starlark.NewBuiltin("struct", starlarkstruct.Make),
		"addr":   starlark.String(ts.URL),
		//"http":   NewModule(),
		"openapi": NewModule(),
		"openapi_path": starlark.String(
			"file://" + filepath.Join(wd, "testdata/swagger.json"),
		),
	}
	starlarkassert.RunTests(t, "testdata/*.star", globals, starlarkthread.AssertOption)
}
