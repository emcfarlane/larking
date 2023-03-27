// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlarkopenapi_test

import (
	"flag"
	"io"
	"net/http"
	"testing"

	"github.com/google/go-replayers/httpreplay"
	"go.starlark.net/starlark"
	_ "gocloud.dev/blob/fileblob"
	"larking.io/starlib"
	"larking.io/starlib/net/starlarkhttp"
)

var record = flag.Bool("record", false, "perform live requests")

func TestOpenAPIV2(t *testing.T) {
	var (
		client   *http.Client
		filename = "testdata/" + t.Name() + ".replay"
	)
	if *record {
		t.Log("recording...")
		rec, err := httpreplay.NewRecorder(filename, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { rec.Close() })
		client = rec.Client()
	} else {
		rpl, err := httpreplay.NewReplayer(filename)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { rpl.Close() })
		client = rpl.Client()
	}

	c := starlarkhttp.NewClient(client)
	c.SetDebug(true)

	starlib.RunTests(t, "testdata/swagger_test.star", starlark.StringDict{
		"addr":   starlark.String("https://petstore.swagger.io/"), //starlark.String(ts.URL),
		"spec":   starlark.String("testdata/swagger.json"),
		"client": c,
	})

}

func TestGet(t *testing.T) {

	var (
		client   *http.Client
		filename = "testdata/" + t.Name() + ".replay"
	)
	if *record {
		t.Log("recording...")
		rec, err := httpreplay.NewRecorder(filename, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer rec.Close()
		client = rec.Client()
	} else {
		rpl, err := httpreplay.NewReplayer(filename)
		if err != nil {
			t.Fatal(err)
		}
		defer rpl.Close()
		client = rpl.Client()
	}

	// "https://petstore.swagger.io/v2/pet/1"
	addr := "https://petstore.swagger.io/v2/pet/findByStatus?status=available"

	rsp, err := client.Get(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status code: %d", rsp.StatusCode)
	}
	t.Logf("status: %s", rsp.Status)
	t.Logf("headers: %v", rsp.Header)

	body, err := io.ReadAll(rsp.Body)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("body: %s", body)
}
