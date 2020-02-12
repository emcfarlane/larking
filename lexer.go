package graphpb

import (
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
	tokenEOF
)

type token struct {
	typ tokenType
	val string
}

func (t token) isEnd() bool {
	return t.typ == tokenError || t.typ == tokenEOF
}

type tokens []token

func (toks tokens) vals() []string {
	ss := make([]string, len(toks))
	for i, tok := range toks {
		ss[i] = tok.val
	}
	return ss
}

type stateFn func(l *lexer) token

type lexer struct {
	state stateFn
	input string
	start int
	pos   int
	width int
}

// token returns the next token from the lexer
func (l *lexer) token() token {
	return l.state(l)
}

func typSet(typs ...tokenType) (set uint) {
	for _, typ := range typs {
		set |= 1 << uint(typ)
	}
	return set
}

// collect groups tokens returning the group and the last oddball
func (l *lexer) collect(allow, ignore []tokenType) (toks tokens, tok token) {
	allowed := typSet(allow...)
	ignored := typSet(ignore...)
	for tok = l.token(); ; tok = l.token() {
		switch {
		case allowed&(1<<uint(tok.typ)) != 0:
			toks = append(toks, tok)
		case ignored&(1<<uint(tok.typ)) != 0:
		default:
			return
		}
	}
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

func (l *lexer) backup() {
	l.pos -= l.width
}

func (l *lexer) emit(typ tokenType) token {
	tok := token{typ: typ, val: l.input[l.start:l.pos]}
	l.start = l.pos
	return tok
}

func lexEOF(l *lexer) token {
	return token{typ: tokenEOF}
}

func lexError(l *lexer) token {
	return token{typ: tokenError, val: l.input[l.pos-l.width : l.pos]}
}

func isValue(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' || r == '-'
}

func (l *lexer) chain(typOne, typTwo tokenType, next stateFn) token {
	l.state = func(l *lexer) token {
		l.state = next
		return l.emit(typTwo)
	}

	l.pos -= l.width
	tok := l.emit(typOne)
	l.pos += l.width
	return tok
}

func lexText(l *lexer) token {
	for {
		r := l.next()
		switch {
		case isValue(r):
			continue
		case r == '/', r == eof:
			l.backup()
			l.state = lexSegment
			return l.emit(tokenValue)
		default:
			l.state = lexError
			return l.emit(tokenValue)
		}
	}
}

func lexFieldPath(l *lexer) token {
	for {
		r := l.next()
		switch {
		case isValue(r):
			continue
		case r == '.':
			return l.chain(tokenValue, tokenDot, lexFieldPath)
		case r == '=':
			return l.chain(tokenValue, tokenEqual, lexSegment)
		case r == '}':
			return l.chain(tokenValue, tokenVariableEnd, lexSegment)
		}
	}
}

func lexSegment(l *lexer) token {
	r := l.next()
	switch {
	case r == '/':
		return l.emit(tokenSlash)
	case unicode.IsLetter(r):
		l.state = lexText
		return l.token()
	case r == '{':
		l.state = lexFieldPath
		return l.emit(tokenVariableStart)
	case r == '}':
		return l.emit(tokenVariableEnd)
	case r == '*':
		rn := l.next()
		if rn == '*' {
			return l.emit(tokenStarStar)
		}
		l.backup()
		return l.emit(tokenStar)
	case r == eof:
		l.state = lexEOF
		return l.token()
	default:
		l.state = lexError
		return l.token()
	}
}
