// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlarkopenapi_test

import (
	"flag"
	"io"
	"net/http"
	"os"
	"testing"

	openapiv2 "github.com/google/gnostic/openapiv2"
	openapiv3 "github.com/google/gnostic/openapiv3"
	surface "github.com/google/gnostic/surface"
	"github.com/google/go-replayers/httpreplay"
	"go.starlark.net/starlark"
	_ "gocloud.dev/blob/fileblob"
	"google.golang.org/protobuf/encoding/prototext"
	"larking.io/starlib"
	"larking.io/starlib/net/starlarkhttp"
)

var (
	record   = flag.Bool("record", false, "perform live requests")
	debug    = flag.Bool("debug", false, "debug client requests")
	generate = flag.Bool("generate", false, "generate models")
)

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
	if *debug {
		c.SetDebug(true)
	}

	starlib.RunTests(t, "testdata/swagger_test.star", starlark.StringDict{
		"addr":   starlark.String("https://petstore.swagger.io/"), //starlark.String(ts.URL),
		"spec":   starlark.String("testdata/v2.0/petstore.json"),
		"client": c,
	})

}

func TestOpenAPIV3(t *testing.T) {
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
	if *debug {
		c.SetDebug(true)
	}

	starlib.RunTests(t, "testdata/openapi_test.star", starlark.StringDict{
		"addr":   starlark.String("https://petstore3.swagger.io/api/v3"),
		"spec":   starlark.String("https://petstore3.swagger.io/api/v3/openapi.json"),
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

func TestModels(t *testing.T) {
	if !*generate {
		t.Skip("skipping model generation")
	}

	{
		refFile := "testdata/v3.0/petstore.json"
		modelFile := "testdata/v3.0/petstore.model.txt"

		b, err := os.ReadFile(refFile)
		if err != nil {
			t.Fatalf("Failed to read file: %+v", err)
		}

		docv3, err := openapiv3.ParseDocument(b)
		if err != nil {
			t.Fatalf("Failed to parse document: %+v", err)
		}

		m, err := surface.NewModelFromOpenAPI3(docv3, refFile)
		if err != nil {
			t.Fatalf("Failed to create model: %+v", err)
		}

		x := prototext.Format(m)
		if err := os.WriteFile(modelFile, []byte(x), 0644); err != nil {
			t.Fatal(err)
		}
	}

	{
		refFile := "testdata/v2.0/petstore.json"
		modelFile := "testdata/v2.0/petstore.model.txt"

		b, err := os.ReadFile(refFile)
		if err != nil {
			t.Fatalf("Failed to read file: %+v", err)
		}

		docv2, err := openapiv2.ParseDocument(b)
		if err != nil {
			t.Fatalf("Failed to parse document: %+v", err)
		}

		m, err := surface.NewModelFromOpenAPI2(docv2, refFile)
		if err != nil {
			t.Fatalf("Failed to create model: %+v", err)
		}

		x := prototext.Format(m)
		if err := os.WriteFile(modelFile, []byte(x), 0644); err != nil {
			t.Fatal(err)
		}
	}
}
