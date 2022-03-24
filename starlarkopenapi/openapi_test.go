// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlarkopenapi

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"os"
	"path/filepath"
	"testing"

	"github.com/emcfarlane/larking/starlarkblob"
	"github.com/emcfarlane/larking/starlarkhttp"
	"github.com/emcfarlane/larking/starlarkthread"
	"github.com/emcfarlane/starlarkassert"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	_ "gocloud.dev/blob/fileblob"
	_ "gocloud.dev/runtimevar/filevar"
)

var record = flag.Bool("record", false, "perform live requests")

type transport struct {
	prefix string
	count  int
	http.RoundTripper
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	reqName := fmt.Sprintf("%s_%d_req.txt", t.prefix, t.count)
	rspName := fmt.Sprintf("%s_%d_rsp.txt", t.prefix, t.count)
	t.count++

	if !*record {
		reqBytes, err := ioutil.ReadFile(reqName)
		if err != nil {
			return nil, err
		}

		rspBytes, err := ioutil.ReadFile(rspName)
		if err != nil {
			return nil, err
		}

		if wantBytes, err := httputil.DumpRequest(req, true); err != nil {
			return nil, err
		} else if cmp := bytes.Compare(reqBytes, wantBytes); cmp != 0 {
			fmt.Println("reqBytes", len(reqBytes), string(reqBytes))
			fmt.Println("wantBytes", len(wantBytes), string(wantBytes))
			return nil, fmt.Errorf("request changed: %d", cmp)
		}

		br := bufio.NewReader(bytes.NewReader(rspBytes))
		return http.ReadResponse(br, req)
	}

	if reqBytes, err := httputil.DumpRequest(req, true); err != nil {
		return nil, err
	} else if err := ioutil.WriteFile(reqName, reqBytes, 0644); err != nil {
		log.Fatal(err)
	}

	rsp, err := t.RoundTripper.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	if rspBytes, err := httputil.DumpResponse(rsp, true); err != nil {
		return nil, err
	} else if err := ioutil.WriteFile(rspName, rspBytes, 0644); err != nil {
		log.Fatal(err)
	}

	return rsp, nil
}

func wrapClient(t *testing.T, name string, client *http.Client) {
	client.Transport = &transport{
		prefix:       name,
		RoundTripper: client.Transport,
	}
}

func TestExecFile(t *testing.T) {
	mux := http.NewServeMux()
	//mux.HandleFunc("/hello", func(w http.ResponseWriter, _ *http.Request) {
	//	fmt.Fprintln(w, "world")
	//})

	// Create a test http server.
	ts := httptest.NewServer(mux)
	defer ts.Close()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Log(wd)

	client := ts.Client()
	wrapClient(t, filepath.Join("testdata", t.Name()), client)

	globals := starlark.StringDict{
		"struct": starlark.NewBuiltin("struct", starlarkstruct.Make),
		"addr":   starlark.String(ts.URL),
		"client": starlarkhttp.MakeClient(client),
		//"http":   NewModule(),
		"openapi": NewModule(),
		//"openapi_path": starlark.String(
		//	"file://" + filepath.Join(wd, "testdata/swagger.json"),
		//),
		"spec_var": starlark.String(
			"file://" + filepath.Join(wd, "testdata/swagger.json"),
		),
		"blob": starlarkblob.NewModule(),
	}
	starlarkassert.RunTests(t, "testdata/*.star", globals, starlarkthread.AssertOption)
}
