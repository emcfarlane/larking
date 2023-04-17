// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package larking

import (
	"fmt"
	"strings"
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

type tokenType uint16

const (
	tokenError         tokenType = 0
	tokenSlash         tokenType = 1 << iota // /
	tokenStar                                // *
	tokenStarStar                            // **
	tokenVariableStart                       // {
	tokenVariableEnd                         // }
	tokenEqual                               // =
	tokenValue                               // a-z A-Z 0-9 - _
	tokenDot                                 // .
	tokenVerb                                // :
	tokenPath                                // a-z A-Z 0-9 . - _ ~ ! $ & ' ( ) * + , ; = @
	tokenEOF
)

type token struct {
	val []byte
	typ tokenType
}

type tokens []token

func (toks tokens) String() string {
	var b strings.Builder
	for _, tok := range toks {
		b.Write(tok.val)
	}
	return b.String()
}

func (toks tokens) index(typ tokenType) int {
	for i, tok := range toks {
		if tok.typ == typ {
			return i
		}
	}
	return -1
}

func (toks tokens) indexAny(s tokenType) int {
	for i, tok := range toks {
		if s&tok.typ != 0 {
			return i
		}
	}
	return -1
}

type lexer struct {
	input []byte
	start int
	pos   int
	width int

	toks tokens
}

const eof = -1

func (l *lexer) next() (r rune) {
	if l.pos >= len(l.input) {
		l.width = 0
		return eof
	}
	if c := l.input[l.pos]; c < utf8.RuneSelf {
		l.width = 1
		r = rune(c)
	} else {
		r, l.width = utf8.DecodeRune(l.input[l.pos:])
	}
	l.pos += l.width
	return r
}

func (l *lexer) current() (r rune) {
	if l.width == 0 {
		return 0
	} else if l.pos > l.width {
		r, _ = utf8.DecodeRune(l.input[l.pos-l.width:])
	} else {
		r, _ = utf8.DecodeRune(l.input)
	}
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

func (l *lexer) emit(typ tokenType) {
	tok := token{typ: typ, val: l.input[l.start:l.pos]}
	l.toks = append(l.toks, tok)
	l.start = l.pos
}

func (l *lexer) errUnexpected() error {
	l.emit(tokenError)
	r := l.current()
	return fmt.Errorf("%v:%v unexpected rune %q", l.pos-l.width, l.pos, r)
}
func (l *lexer) errShort() error {
	l.emit(tokenError)
	r := l.current()
	return fmt.Errorf("%v:%v short read %q", l.pos-l.width, l.pos, r)
}

func isValue(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' || r == '-'
}

func isPath(r rune) bool {
	return isValue(r) || r == '.' || r == '~' || r == '!' || r == '$' ||
		r == '&' || r == '\'' || r == '(' || r == ')' || r == '*' ||
		r == '+' || r == ',' || r == ';' || r == '=' || r == '@'
}

func lexValue(l *lexer) error {
	if i := l.acceptRun(isValue); i == 0 {
		return l.errShort()
	}
	l.emit(tokenValue)
	return nil
}

func lexFieldPath(l *lexer) error {
	if err := lexValue(l); err != nil {
		return err
	}
	for {
		if r := l.next(); r != '.' {
			l.backup() // unknown
			return nil
		}
		l.emit(tokenDot)
		if err := lexValue(l); err != nil {
			return err
		}
	}
}

func lexVerb(l *lexer) error {
	if err := lexValue(l); err != nil {
		return err
	}
	if r := l.next(); r == eof {
		l.emit(tokenEOF)
		return nil
	}
	return l.errUnexpected()
}

func lexVariable(l *lexer) error {
	r := l.next()
	if r != '{' {
		return l.errUnexpected()
	}
	l.emit(tokenVariableStart)
	if err := lexFieldPath(l); err != nil {
		return err
	}

	r = l.next()
	if r == '=' {
		l.emit(tokenEqual)

		if err := lexSegments(l); err != nil {
			return err
		}
		r = l.next()
	}

	if r != '}' {
		return l.errUnexpected()
	}
	l.emit(tokenVariableEnd)
	return nil
}

func lexSegment(l *lexer) error {
	r := l.next()
	switch {
	case unicode.IsLetter(r):
		if i := l.acceptRun(isValue); i == 0 {
			return l.errShort()
		}
		l.emit(tokenValue)
		return nil
	case r == '*':
		rn := l.next()
		if rn == '*' {
			l.emit(tokenStarStar)
			return nil
		}
		l.backup()
		l.emit(tokenStar)
		return nil
	case r == '{':
		l.backup()
		return lexVariable(l)
	default:
		return l.errUnexpected()
	}
}

func lexSegments(l *lexer) error {
	for {
		if err := lexSegment(l); err != nil {
			return err
		}
		if r := l.next(); r != '/' {
			l.backup() // unknown
			return nil
		}
		l.emit(tokenSlash)
	}
}

func lexTemplate(l *lexer) error {
	if r := l.next(); r != '/' {
		return l.errUnexpected()
	}
	l.emit(tokenSlash)
	if err := lexSegments(l); err != nil {
		return err
	}

	switch r := l.next(); r {
	case ':':
		l.emit(tokenVerb)
		return lexVerb(l)
	case eof:
		l.emit(tokenEOF)
		return nil
	default:
		return l.errUnexpected()
	}
}

func lexPathSegment(l *lexer) error {
	if i := l.acceptRun(isPath); i == 0 {
		return l.errShort()
	}
	l.emit(tokenPath)
	return nil
}

// lexPath emits all tokenSlash, tokenVerb and the rest as tokenPath
func lexPath(l *lexer) error {
	for {
		switch r := l.next(); r {
		case '/':
			l.emit(tokenSlash)
			if err := lexPathSegment(l); err != nil {
				return err
			}
		case ':':
			l.emit(tokenVerb)
			if err := lexPathSegment(l); err != nil {
				return err
			}
		case eof:
			l.emit(tokenEOF)
			return nil
		default:
			return l.errUnexpected()
		}
	}
}
