package starlib

import (
	"context"
	"net/url"
	"testing"

	"github.com/emcfarlane/starlarkassert"
	"go.starlark.net/starlark"
	"larking.io/starlib/starlarkthread"
)

func nameTestOption(_ testing.TB, thread *starlark.Thread) func() {
	_, err := url.ParseRequestURI(thread.Name)
	if err != nil {
		u := &url.URL{
			Scheme: "file",
			Host:   ".",
			Path:   "/",
		}
		q := u.Query()
		q.Set("metadata", "skip")
		q.Set("key", thread.Name)
		u.RawQuery = q.Encode()
		thread.Name = u.String()
	}
	return nil
}

func ctxTestOption(_ testing.TB, thread *starlark.Thread) func() {
	ctx, cancel := context.WithCancel(context.Background())
	starlarkthread.SetContext(thread, ctx)
	return cancel
}

// RunTests calls starlarkassert.RunTests with options for larking libraries.
// To use add it to a Test function:
//
//	func TestStarlark(t *testing.T) {
//		starlib.RunTests(b, "testdata/*.star", nil)
//	}
func RunTests(t *testing.T, pattern string, globals starlark.StringDict, opts ...starlarkassert.TestOption) {
	t.Helper()

	g := NewGlobals()
	for key, val := range globals {
		g[key] = val
	}
	loader := NewLoader(g)

	opts = append([]starlarkassert.TestOption{
		starlarkthread.AssertOption,
		starlarkassert.WithLoad(loader.Load),
		nameTestOption,
		ctxTestOption,
	}, opts...)

	starlarkassert.RunTests(t, pattern, g, opts...)
}

// RunBenches calls starlarkassert.RunBenches with options for larking libraries.
// To use add it to a Benchmark function:
//
//	func BenchmarkStarlark(b *testing.B) {
//		starlib.RunBenches(b, "testdata/*.star", nil)
//	}
func RunBenches(b *testing.B, pattern string, globals starlark.StringDict, opts ...starlarkassert.TestOption) {
	b.Helper()

	g := NewGlobals()
	for key, val := range globals {
		g[key] = val
	}
	loader := NewLoader(g)

	opts = append([]starlarkassert.TestOption{
		starlarkthread.AssertOption,
		starlarkassert.WithLoad(loader.Load),
		nameTestOption,
		ctxTestOption,
	}, opts...)

	starlarkassert.RunBenches(b, pattern, g, opts...)
}
