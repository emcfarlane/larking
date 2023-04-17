// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package larking

import (
	"bytes"
	"testing"
)

func TestLexer(t *testing.T) {
	tests := []struct {
		name    string
		tmpl    string
		want    tokens
		wantErr bool
	}{{
		name: "one",
		tmpl: "/v1/messages/{name=name/*}",
		want: tokens{
			{typ: tokenSlash, val: []byte("/")},
			{typ: tokenValue, val: []byte("v1")},
			{typ: tokenSlash, val: []byte("/")},
			{typ: tokenValue, val: []byte("messages")},
			{typ: tokenSlash, val: []byte("/")},
			{typ: tokenVariableStart, val: []byte("{")},
			{typ: tokenValue, val: []byte("name")},
			{typ: tokenEqual, val: []byte("=")},
			{typ: tokenValue, val: []byte("name")},
			{typ: tokenSlash, val: []byte("/")},
			{typ: tokenStar, val: []byte("*")},
			{typ: tokenVariableEnd, val: []byte("}")},
			{typ: tokenEOF, val: []byte("")},
		},
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			l := &lexer{
				input: []byte(tt.tmpl),
			}
			err := lexTemplate(l)
			if tt.wantErr {
				if err == nil {
					t.Error("wanted failure but succeeded")
				}
				return
			}
			if err != nil {
				t.Error(err)
			}
			if n, m := len(tt.want), len(l.toks); n != m {
				t.Errorf("mismatch length %v != %v:\n\t%v\n\t%v", n, m, tt.want, l.toks)
				return
			}
			for i, want := range tt.want {
				tok := l.toks[i]
				if want.typ != tok.typ || !bytes.Equal(want.val, tok.val) {
					t.Errorf("%d: %v != %v", i, tok, want)
				}
			}
		})
	}
}

func BenchmarkLexer(b *testing.B) {
	input := []byte("/v1/books/1/shevles/1:read")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l := &lexer{
			input: input,
		}
		lexPath(l)
	}
}
