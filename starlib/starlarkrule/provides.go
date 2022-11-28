// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlarkrule

import (
	"fmt"
	"strings"

	"go.starlark.net/starlark"
)

type Provides struct {
	attrs  []*Attr
	frozen bool
}

func (p *Provides) String() string {
	buf := new(strings.Builder)
	buf.WriteString("attrs")
	buf.WriteByte('(')
	for i, attr := range p.attrs {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(attr.String())
	}
	buf.WriteByte(')')
	return buf.String()
}
func (p *Provides) Type() string { return "rule.provides" }
func (p *Provides) Freeze() {
	if p.frozen {
		return
	}
	p.frozen = true
	for _, a := range p.attrs {
		a.Freeze()
	}

}
func (p *Provides) Truth() starlark.Bool  { return true }
func (p *Provides) Hash() (uint32, error) { return 0, nil } // not hashable

func MakeProvides(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(kwargs) > 0 {
		return nil, fmt.Errorf("%s: unexpected keyword arguments", fnname)
	}

	index := make(map[string]int)
	var attrs []*Attr
	for i, arg := range args {
		a, ok := arg.(*Attr)
		if !ok {
			return nil, fmt.Errorf("unexpected attribute value type: %q", arg.Type())
		}
		t := a.Type()
		if _, ok := index[t]; ok {
			return nil, fmt.Errorf("duplicate provider type: %q", t)
		}
		attrs = append(attrs, a)
		index[t] = i
	}

	return &Provides{
		attrs: attrs,
	}, nil
}
