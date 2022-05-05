// https://pkg.go.dev/archive/zip
package starlarkzip

import "archive/zip"

type Writer struct {
	w *zip.Writer
}

// w = writer(output)
// f = w.create(name)
// f.write(string|bytes|reader)
//
// h = header(...)
//
// r = reader(input)
// r.comment
// for (hdr, rdr) in r:
