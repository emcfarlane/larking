// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package larking

// Support for gRPC-web
// https://github.com/grpc/grpc/blob/master/doc/PROTOCOL-WEB.md
// https://github.com/grpc/grpc/blob/master/doc/PROTOCOL-HTTP2.md

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	grpcBase    = "application/grpc"
	grpcWeb     = "application/grpc-web"
	grpcWebText = "application/grpc-web-text"
)

// isWebRequest checks for gRPC Web headers.
func isWebRequest(r *http.Request) (typ string, enc string, ok bool) {
	ct := r.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/grpc-web") || r.Method != http.MethodPost {
		return typ, enc, false
	}
	typ, enc, ok = strings.Cut(ct, "+")
	if !ok {
		enc = "proto"
	}
	ok = typ == grpcWeb || typ == grpcWebText
	return typ, enc, ok
}

type webWriter struct {
	w           http.ResponseWriter
	resp        io.Writer
	seenHeaders map[string]bool
	typ         string // grpcWeb or grpcWebText
	enc         string // proto or json
	wroteHeader bool
	wroteResp   bool
}

func newWebWriter(w http.ResponseWriter, typ, enc string) *webWriter {
	var resp io.Writer = w
	if typ == grpcWebText {
		resp = base64.NewEncoder(base64.StdEncoding, resp)

	}

	return &webWriter{
		w:    w,
		typ:  typ,
		enc:  enc,
		resp: resp,
	}
}

func (w *webWriter) seeHeaders() {
	hdr := w.Header()
	hdr.Set("Content-Type", w.typ+"+"+w.enc) // override content-type

	keys := make(map[string]bool, len(hdr))
	for k := range hdr {
		if strings.HasPrefix(k, http.TrailerPrefix) {
			continue
		}
		keys[k] = true
	}
	w.seenHeaders = keys
	w.wroteHeader = true
}

func (w *webWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.seeHeaders()
	}
	return w.resp.Write(b)
}

func (w *webWriter) Header() http.Header { return w.w.Header() }

func (w *webWriter) WriteHeader(code int) {
	w.seeHeaders()
	w.w.WriteHeader(code)
}

func (w *webWriter) Flush() {
	if w.wroteHeader || w.wroteResp {
		if f, ok := w.w.(http.Flusher); ok {
			f.Flush()
		}
	}
}

func (w *webWriter) writeTrailer() error {
	hdr := w.Header()

	tr := make(http.Header, len(hdr)-len(w.seenHeaders)+1)
	for key, val := range hdr {
		if w.seenHeaders[key] {
			continue
		}
		key = strings.TrimPrefix(key, http.TrailerPrefix)
		// https://github.com/grpc/grpc/blob/master/doc/PROTOCOL-WEB.md#protocol-differences-vs-grpc-over-http2
		tr[strings.ToLower(key)] = val
	}

	var buf bytes.Buffer
	if err := tr.Write(&buf); err != nil {
		return err
	}

	head := []byte{1 << 7, 0, 0, 0, 0} // MSB=1 indicates this is a trailer data frame.
	binary.BigEndian.PutUint32(head[1:5], uint32(buf.Len()))
	if _, err := w.Write(head); err != nil {
		return err
	}
	if _, err := w.Write(buf.Bytes()); err != nil {
		return err
	}
	return nil
}

func (w *webWriter) flushWithTrailer() {
	// Write trailers only if message has been sent.
	if w.wroteHeader || w.wroteResp {
		if err := w.writeTrailer(); err != nil {
			return // nothing
		}
	}
	w.Flush()
}

type readCloser struct {
	io.Reader
	io.Closer
}

func (m *Mux) serveGRPCWeb(w http.ResponseWriter, r *http.Request) {
	typ, enc, ok := isWebRequest(r)
	if !ok {
		msg := fmt.Sprintf("invalid gRPC-Web content type: %v", r.Header.Get("Content-Type"))
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	// TODO: Check for websocket request and upgrade.
	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "unimplemented websocket support", http.StatusInternalServerError)
		return
	}

	r.ProtoMajor = 2
	r.ProtoMinor = 0

	hdr := r.Header
	hdr.Del("Content-Length")
	hdr.Set("Content-Type", grpcBase+"+"+enc)

	if typ == grpcWebText {
		body := base64.NewDecoder(base64.StdEncoding, r.Body)
		r.Body = readCloser{body, r.Body}
	}

	ww := newWebWriter(w, typ, enc)
	m.serveGRPC(ww, r)
	ww.flushWithTrailer()
}
