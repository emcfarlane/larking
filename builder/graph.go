package builder

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"larking.io/starlib"
	"larking.io/starlib/starlarkrule"
	"larking.io/starlib/starlarkthread"
	"go.starlark.net/starlark"
)

// TODO: build dot-graph
// https://graphviz.org/Gallery/directed/bazel.html
// https://graphviz.org/Gallery/directed/go-package.html

// https://github.com/google/pprof/blob/83db2b799d1f74c40857232cb5eb4c60379fe6c2/internal/driver/webui.go#L332
func dotToSvg(dot []byte) ([]byte, error) {
	cmd := exec.Command("dot", "-Tsvg")
	out := &bytes.Buffer{}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = bytes.NewBuffer(dot), out, os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	// Fix dot bug related to unquoted ampersands.
	svg := bytes.ReplaceAll(out.Bytes(), []byte("&;"), []byte("&amp;;"))

	// Cleanup for embedding by dropping stuff before the <svg> start.
	if pos := bytes.Index(svg, []byte("<svg")); pos >= 0 {
		svg = svg[pos:]
	}
	return svg, nil
}

func generateDot(a *starlarkrule.Action) ([]byte, error) {
	var (
		buf  bytes.Buffer
		tabs = 0
	)
	p := func(ss ...string) {
		for i := 0; i < tabs; i++ {
			buf.WriteRune('\t')
		}
		for _, s := range ss {
			buf.WriteString(s)
		}
		buf.WriteRune('\n')
	}
	q := `"`

	p(`digraph laze {`)
	tabs += 1
	p(`fontname="Helvetica,Arial,sans-serif"`)
	p(`node [fontname="Helvetica,Arial,sans-serif"]`)
	p(`edge [fontname="Helvetica,Arial,sans-serif"]`)
	p(`node [shape=box];`)
	//p(`rankdir="LR"`)

	deps := []*starlarkrule.Action{a}
	for n := len(deps); n > 0; n = len(deps) {
		a, deps = deps[n-1], deps[:n-1] // pop

		p(q, a.Key(), q)

		deps = append(deps, a.Deps...)
		for _, at := range a.Deps {
			p(q, at.Key(), q, " -> ", q, a.Key(), q)
		}
	}

	tabs -= 1
	p(`}`)

	return buf.Bytes(), nil
}

var isLocalhost = map[string]bool{
	"localhost": true,
	"127.0.0.1": true,
	"[::1]":     true,
	"::1":       true,
}

func Serve(l net.Listener) error { //addr string) error {
	host, _, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		return err
	}

	dir := ""
	label, err := starlarkrule.ParseRelativeLabel("file://./?metadata=skip", dir)
	if err != nil {
		return err
	}

	b, err := starlarkrule.NewBuilder(label)
	if err != nil {
		return err
	}

	globals := starlib.NewGlobals()
	loader := starlib.NewLoader(globals)

	resources := starlarkthread.ResourceStore{} // resources
	defer resources.Close()

	isLocal := isLocalhost[host]
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isLocal {
			// Only allow local clients
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil || !isLocalhost[host] {
				http.Error(w, "permission denied", http.StatusForbidden)
				return
			}
		}

		name := strings.TrimPrefix(r.URL.Path, "/graph/")
		l, err := label.Parse(name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		ctx := r.Context()
		thread := &starlark.Thread{
			Name: l.String(),
			Load: loader.Load,
		}
		starlarkthread.SetResourceStore(thread, &resources)
		starlarkthread.SetContext(thread, ctx)

		action, err := b.Build(thread, label)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		dot, err := generateDot(action)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Println("------ dot -------")
		fmt.Println(string(dot))
		fmt.Println("------ dot -------")

		svg, err := dotToSvg(dot)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var buf bytes.Buffer

		buf.WriteString(`<html>
    <head>
        <title>Laze</title>
    </head>
    <body>`)
		buf.Write(svg)
		buf.WriteString(`    </body>
</html>`)

		w.Header().Set("Content-Type", "text/html")
		if _, err := io.Copy(w, &buf); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

	})

	mux := http.NewServeMux()
	//mux.Handle("/", handler)
	mux.Handle("/graph/", handler)
	s := &http.Server{Handler: mux}
	return s.Serve(l)
}
