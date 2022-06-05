package builder

import (
	"context"
	"io/ioutil"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
)

func TestGraph(t *testing.T) {
	ctx := logr.NewContext(context.Background(), testr.New(t))
	a, err := Build(ctx, "testdata/archive/helloc.tar.gz", func(o *buildOptions) {
		o.run = false
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Log("action", a)
	t.Log("deps", a.Deps)
	if len(a.Deps) != 1 {
		t.Errorf("missing deps: %v", a.Deps)
	}

	dot, err := generateDot(a)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(dot))

	svg, err := dotToSvg(dot)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(svg))

	if len(svg) == 0 {
		t.Error("missing svg")
	}

	if err := ioutil.WriteFile("graph.svg", svg, 0666); err != nil {
		t.Fatal(err)
	}
}