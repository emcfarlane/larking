package laze

import (
	"bytes"
	"os"
	"os/exec"
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
	svg := bytes.Replace(out.Bytes(), []byte("&;"), []byte("&amp;;"), -1)

	// Cleanup for embedding by dropping stuff before the <svg> start.
	if pos := bytes.Index(svg, []byte("<svg")); pos >= 0 {
		svg = svg[pos:]
	}
	return svg, nil
}
