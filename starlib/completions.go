// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlib

import (
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"go.starlark.net/starlark"
)

// finxPrefix assumes sorted arrays of keys
func findPrefix(line string, depth int, pfx string, keyss ...[]string) (c []string) {
	pfx = strings.TrimSpace(pfx) // ignore any whitespacing
	for _, keys := range keyss {
		i := sort.SearchStrings(keys, pfx)
		j := i
		for ; j < len(keys); j++ {
			if !strings.HasPrefix(keys[j], pfx) {
				break
			}
		}
		c = append(c, keys[i:j]...)
	}
	if len(keyss) > 1 {
		sort.Strings(c)
	}

	// Add line start
	for i := range c {
		c[i] = line[:depth] + c[i]
	}
	return c
}

// An Args is a starlark Callable with arguments.
type Args interface {
	starlark.Callable
	ArgNames() []string
}

// Completer is an experimental autocompletion for starlark lines.
// TODO: drop and switch to a proper language server.
type Completer struct {
	starlark.StringDict
}

type typ int

const (
	unknown typ = iota - 1
	root        //
	dot         // .
	brack       // []
	paren       // ()
	brace       // {}
)

func (t typ) String() string {
	switch t {
	case dot:
		return "."
	case brack:
		return "["
	case paren:
		return "("
	case brace:
		return "{"
	default:
		return "?"
	}
}

func enclosed(line string) (typ, int) {
	k := len(line)
	var parens, bracks, braces int
	for size := 0; k > 0; k -= size {
		var r rune
		r, size = utf8.DecodeLastRuneInString(line[:k])
		switch r {
		case '(':
			parens += 1
		case ')':
			parens -= 1
		case '[':
			bracks += 1
		case ']':
			bracks -= 1
		case '{':
			braces += 1
		case '}':
			braces -= 1
		}
		if parens > 0 {
			return paren, k - size
		}
		if bracks > 0 {
			return brack, k - size
		}
		if braces > 0 {
			return brace, k - size
		}
	}
	return unknown, -1
}

// Complete tries to resolve a starlark line variable to global named values.
// TODO: use a proper parser to resolve values.
func (c Completer) Complete(line string) (values []string) {
	if strings.Count(line, " ") == len(line) {
		// tab complete indent
		return []string{strings.Repeat(" ", (len(line)/4)*4+4)}
	}

	type x struct {
		typ   typ
		value string
		depth int
	}

	var xs []x

	i := len(line)
	j := i

Loop:
	for size := 0; i > 0; i -= size {
		var r rune
		switch r, size = utf8.DecodeLastRuneInString(line[:i]); r {
		case '.': // attr
			xs = append(xs, x{dot, line[i:j], i})
		case '[': // index
			xs = append(xs, x{brack, line[i:j], i})
		case '(': // functions
			xs = append(xs, x{paren, line[i:j], i})
		case ' ', ',':
			typ, k := enclosed(line[:i-size])

			// Use ArgNames as possible completion
			if typ == paren {
				xs = append(xs, x{typ, line[i:j], i})
				i, j = k, k
				continue // loop
			}

			break Loop
		case ';', '=', '{', '}':
			break Loop // EOF
		default:
			continue // capture
		}
		j = i - size
	}
	xs = append(xs, x{root, line[i:j], i})

	var cursor starlark.Value
	for i := len(xs) - 1; i >= 0; i-- {
		x := xs[i]

		switch x.typ {
		case root:
			if i == 0 {
				keys := [][]string{c.Keys(), starlark.Universe.Keys()}
				return findPrefix(line, x.depth, x.value, keys...)
			}

			if g := c.StringDict[x.value]; g != nil {
				cursor = g
			} else if u := starlark.Universe[x.value]; u != nil {
				cursor = u
			}
		case dot:
			v, ok := cursor.(starlark.HasAttrs)
			if !ok {
				return
			}

			if i == 0 {
				return findPrefix(line, x.depth, x.value, v.AttrNames())
			}

			p, err := v.Attr(x.value)
			if p == nil || err != nil {
				return
			}
			cursor = p
		case brack:
			if i != 0 {
				// TODO: resolve arg? fmt.Printf("TODO: resolve arg %s\n", x.value)
				return
			}

			if strings.HasPrefix(x.value, "\"") {
				v, ok := cursor.(starlark.IterableMapping)
				if !ok {
					return
				}

				iter := v.Iterate()
				var keys []string
				var p starlark.Value
				for iter.Next(&p) {
					s, ok := starlark.AsString(p)
					if !ok {
						continue // skip
					}
					keys = append(keys, strconv.Quote(s)+"]")
				}
				return findPrefix(line, x.depth, x.value, keys)
			}
			keys := [][]string{c.Keys(), starlark.Universe.Keys()}
			return findPrefix(line, x.depth, x.value, keys...)

		case paren:
			if i != 0 {
				return // Functions aren't evalutated
			}

			keys := [][]string{c.Keys(), starlark.Universe.Keys()}
			v, ok := cursor.(Args)
			if ok {
				args := v.ArgNames()
				for i := range args {
					args[i] = args[i] + " = "
				}
				keys = append(keys, args)
			}

			return findPrefix(line, x.depth, x.value, keys...)
		default:
			return
		}
	}
	return
}
