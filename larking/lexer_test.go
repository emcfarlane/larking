// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package larking

import (
	"testing"
	"unsafe"
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
			{typ: tokenSlash, val: "/"},
			{typ: tokenValue, val: "v1"},
			{typ: tokenSlash, val: "/"},
			{typ: tokenValue, val: "messages"},
			{typ: tokenSlash, val: "/"},
			{typ: tokenVariableStart, val: "{"},
			{typ: tokenValue, val: "name"},
			{typ: tokenEqual, val: "="},
			{typ: tokenValue, val: "name"},
			{typ: tokenSlash, val: "/"},
			{typ: tokenStar, val: "*"},
			{typ: tokenVariableEnd, val: "}"},
			{typ: tokenEOF, val: ""},
		},
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			l := &lexer{
				input: tt.tmpl,
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
			if n, m := len(tt.want), len(l.tokens()); n != m {
				t.Errorf("mismatch length %v != %v:\n\t%v\n\t%v", n, m, tt.want, l.tokens())
				return
			}
			for i, want := range tt.want {
				tok := l.toks[i]
				if want.typ != tok.typ || want.val != tok.val {
					t.Errorf("%d: %v != %v", i, tok, want)
				}
			}
		})
	}
}

func BenchmarkLexer(b *testing.B) {
	var l lexer
	input := "/v1/books/1/shevles/1:read"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l = lexer{input: input}
		lexPath(&l)
	}
	b.StopTimer()
	if n := l.len; n != 13 {
		b.Errorf("expected %d tokens: %d", 7, n)
	}
	b.Logf("%v", unsafe.Sizeof(l))
}
