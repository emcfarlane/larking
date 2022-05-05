// https://pkg.go.dev/archive/tar
package starlarktar

import "archive/tar"

type Writer struct {
	w *tar.Writer
}

// w = writer(output)
// w.write(string|bytes|reader)
// w.write_header(hdr)
//
// h = header(...)
//
// r = reader(input)
// for (hdr, rdr) in r:
