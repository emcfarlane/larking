// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package larking

import (
	"fmt"
	"unicode"
	"unicode/utf8"
)

// ### Path template syntax
//
//     Template = "/" Segments [ Verb ] ;
//     Segments = Segment { "/" Segment } ;
//     Segment  = "*" | "**" | LITERAL | Variable ;
//     Variable = "{" FieldPath [ "=" Segments ] "}" ;
//     FieldPath = IDENT { "." IDENT } ;
//     Verb     = ":" LITERAL ;

type tokenType int

const (
	tokenError         = iota
	tokenSlash         // /
	tokenStar          // *
	tokenStarStar      // **
	tokenVariableStart // {
	tokenVariableEnd   // }
	tokenEqual         // =
	tokenValue         // -_0-9a-zA-Z
	tokenDot           // .
	tokenVerb          // :
	tokenEOF
)

type token struct {
	typ tokenType
	val string
}

func (t token) String() string {
	return fmt.Sprintf("(%d) %s", t.typ, t.val)
}

type tokens []token

func (toks tokens) vals() []string {
	ss := make([]string, len(toks))
	for i, tok := range toks {
		ss[i] = tok.val
	}
	return ss
}

type stateFn func(l *lexer) error           // stateFn holds the state of the lexer.
type parseFn func(t token) (parseFn, error) // parseFn holds the state of the parser.

type lexer struct {
	parse parseFn
	input string
	start int
	pos   int
	width int
}

func typSet(typs ...tokenType) (set uint) {
	for _, typ := range typs {
		set |= 1 << uint(typ)
	}
	return set
}

// collect groups tokens returning the group and the last oddball
func collect(allow, ignore []tokenType, tokenFn func(tokens, token) (parseFn, error)) parseFn {
	allowed := typSet(allow...)
	ignored := typSet(ignore...)

	var ts tokens
	var pFn parseFn
	pFn = func(t token) (parseFn, error) {
		switch {
		case allowed&(1<<uint(t.typ)) != 0:
			ts = append(ts, t)
		case ignored&(1<<uint(t.typ)) != 0:
		default:
			return tokenFn(ts, t)
		}
		return pFn, nil

	}
	return pFn
}

const eof = -1

func (l *lexer) next() (r rune) {
	if l.pos >= len(l.input) {
		l.width = 0
		return eof
	}
	r, l.width = utf8.DecodeRuneInString(l.input[l.pos:])
	l.pos += l.width
	return r
}

func (l *lexer) current() (r rune) {
	if l.width == 0 {
		return 0
	}
	r, _ = utf8.DecodeRuneInString(l.input[l.pos-l.width:])
	return r
}

func (l *lexer) backup() {
	l.pos -= l.width
}

func (l *lexer) acceptRun(isValid func(r rune) bool) int {
	var i int
	for isValid(l.next()) {
		i++
	}
	l.backup()
	return i
}

func (l *lexer) emit(typ tokenType) error {
	tok := token{typ: typ, val: l.input[l.start:l.pos]}
	parse, err := l.parse(tok)
	if err != nil {
		return err
	}
	l.parse = parse
	l.start = l.pos
	return nil
}

func (l *lexer) emitAndRun(typ tokenType, nextState stateFn) error {
	if err := l.emit(typ); err != nil {
		return err
	}
	return nextState(l)
}

func (l *lexer) errUnexpected() error {
	r := l.current()
	return fmt.Errorf("%v:%v unexpected rune %q", l.pos-l.width, l.pos, r)
}
func (l *lexer) errShort() error {
	r := l.current()
	return fmt.Errorf("%v:%v short read %q", l.pos-l.width, l.pos, r)
}

func lexEOF(l *lexer) error {
	return l.emit(tokenEOF)
}
func lexError(l *lexer) error {
	return l.emit(tokenError)
}

func isValue(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' || r == '-'
}

func lexFieldPath(l *lexer) error {
	if i := l.acceptRun(isValue); i == 0 {
		return l.errShort()
	}
	if err := l.emit(tokenValue); err != nil {
		return err
	}

	r := l.next()
	if r == '.' {
		return l.emitAndRun(tokenDot, lexFieldPath)
	}
	l.backup() // unknown
	return nil
}

func lexVerb(l *lexer) error {
	if i := l.acceptRun(isValue); i == 0 {
		return l.errShort()
	}

	r := l.next()
	switch r {
	case eof:
		l.backup()
		return l.emitAndRun(tokenValue, lexEOF)
	default:
		l.backup()
		return l.emitAndRun(tokenValue, lexError)
	}
}

func lexVariable(l *lexer) error {
	r := l.next()
	if r != '{' {
		return l.errUnexpected()
	}
	if err := l.emit(tokenVariableStart); err != nil {
		return nil
	}

	if err := lexFieldPath(l); err != nil {
		return err
	}

	r = l.next()
	if r == '=' {
		if err := l.emit(tokenEqual); err != nil {
			return err
		}

		if err := lexSegments(l); err != nil {
			return err
		}
		r = l.next()
	}

	if r != '}' {
		return l.errUnexpected()
	}
	return l.emit(tokenVariableEnd)
}

func lexSegment(l *lexer) error {
	r := l.next()
	switch {
	case unicode.IsLetter(r):
		if i := l.acceptRun(isValue); i == 0 {
			return l.errShort()
		}
		return l.emit(tokenValue)
	case r == '*':
		rn := l.next()
		if rn == '*' {
			return l.emit(tokenStarStar)
		}
		l.backup()
		return l.emit(tokenStar)
	case r == '{':
		l.backup()
		return lexVariable(l)
	default:
		// What
		return l.errUnexpected()
	}
}

func lexSegments(l *lexer) error {
	if err := lexSegment(l); err != nil {
		return err
	}
	r := l.next()
	if r == '/' {
		return l.emitAndRun(tokenSlash, lexSegments)
	}
	l.backup() // unknown
	return nil
}

func lexTemplate(l *lexer) error {
	r := l.next()
	if r != '/' {
		return fmt.Errorf("unexpected token %q", r)
	}
	if err := l.emitAndRun(tokenSlash, lexSegments); err != nil {
		return err
	}

	r = l.next()
	switch r {
	case ':':
		return l.emitAndRun(tokenVerb, lexVerb)
	case eof:
		return lexEOF(l)
	default:
		return l.errUnexpected()
	}
}
