package starlib

import (
	"testing"

	"github.com/emcfarlane/larking/starlib/starlarkthread"
	"github.com/emcfarlane/starlarkassert"
	"go.starlark.net/starlark"
)

// RunTests calls starlarkassert.RunTests with options for larking libraries.
// To use add it to a Test function:
//
// 	func TestStarlark(t *testing.T) {
// 		starlib.RunTests(b, "testdata/*.star", nil)
// 	}
//
func RunTests(t *testing.T, pattern string, globals starlark.StringDict) {
	t.Helper()

	g := NewGlobals()
	for key, val := range globals {
		g[key] = val
	}
	loader := NewLoader(g)

	starlarkassert.RunTests(
		t, pattern, g,
		starlarkthread.AssertOption,
		starlarkassert.WithLoad(loader.Load),
		func(_ testing.TB, thread *starlark.Thread) func() {
			thread.Name = "file://./?metadata=skip"
			return nil
		},
	)
}

// RunBenches calls starlarkassert.RunBenches with options for larking libraries.
// To use add it to a Benchmark function:
//
// 	func BenchmarkStarlark(b *testing.B) {
// 		starlib.RunBenches(b, "testdata/*.star", nil)
// 	}
//
func RunBenches(b *testing.B, pattern string, globals starlark.StringDict) {
	b.Helper()

	g := NewGlobals()
	for key, val := range globals {
		g[key] = val
	}
	loader := NewLoader(g)

	starlarkassert.RunBenches(
		b, pattern, g,
		starlarkthread.AssertOption,
		starlarkassert.WithLoad(loader.Load),
		func(_ testing.TB, thread *starlark.Thread) func() {
			thread.Name = "file://./?metadata=skip"
			return nil
		},
	)
}
