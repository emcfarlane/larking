// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package larking

import (
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
			{tokenSlash, "/"},
			{tokenValue, "v1"},
			{tokenSlash, "/"},
			{tokenValue, "messages"},
			{tokenSlash, "/"},
			{tokenVariableStart, "{"},
			{tokenValue, "name"},
			{tokenEqual, "="},
			{tokenValue, "name"},
			{tokenSlash, "/"},
			{tokenStar, "*"},
			{tokenVariableEnd, "}"},
			{tokenEOF, ""},
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
			if n, m := len(tt.want), len(l.toks); n != m {
				t.Errorf("mismatch length %v != %v:\n\t%v\n\t%v", n, m, tt.want, l.toks)
				return
			}
			for i, want := range tt.want {
				tok := l.toks[i]
				if want != tok {
					t.Errorf("%d: %v != %v", i, tok, want)
				}
			}
		})
	}
}
