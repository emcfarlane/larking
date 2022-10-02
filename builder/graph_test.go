package builder

import (
	"context"
	"errors"
	"os"
	"os/exec"
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
		if errors.Is(err, exec.ErrNotFound) {
			t.Log("skipping test, dot not available")
			t.Skip()
			return
		}
		t.Fatal(err)
	}
	t.Log(string(svg))

	if len(svg) == 0 {
		t.Error("missing svg")
	}

	if err := os.WriteFile("graph.svg", svg, 0666); err != nil {
		t.Fatal(err)
	}
}
