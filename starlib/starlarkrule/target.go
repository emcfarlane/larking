// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlarkrule

import (
	"fmt"
	"net/url"
	"strings"

	"go.starlark.net/starlark"
)

// Target is defined by a call to a rule.
type Target struct {
	Label  *Label
	Rule   *Rule
	Kwargs *Kwargs
}

func NewTarget(
	source *Label,
	rule *Rule,
	kwargs *Kwargs,
) (*Target, error) {

	x, err := kwargs.Attr("name")
	if err != nil {
		return nil, err
	}
	name, ok := starlark.AsString(x)
	if !ok {
		return nil, fmt.Errorf("invalid name for target: %v", x)
	}

	l, err := source.Parse(name)
	if err != nil {
		return nil, err
	}

	return &Target{
		Label:  l,
		Rule:   rule,
		Kwargs: kwargs,
	}, nil
}

func (t *Target) Clone() *Target {
	return &Target{
		Label:  t.Label,          // immutable?
		Rule:   t.Rule,           // immuatble
		Kwargs: t.Kwargs.Clone(), // cloned
	}
}

//// TODO?
//func (t *Target) Hash() (uint32, error) {
//	return 0, fmt.Errorf("unhashable type: %s", t.Type())
//}

func (t *Target) String() string {
	var buf strings.Builder
	buf.WriteString(t.Type())
	buf.WriteRune('(')

	buf.WriteString(t.Label.String())
	for i, n := 0, len(t.Kwargs.Values); i < n; i++ {
		if i == 0 {
			buf.WriteRune('?')
		} else {
			buf.WriteRune('&')
		}
		kv := t.Kwargs.Values[i]

		buf.WriteString(kv.Name) // already escaped
		buf.WriteRune('=')
		buf.WriteString(url.QueryEscape(kv.Value.String()))
	}

	buf.WriteRune(')')
	return buf.String()
}
func (t *Target) Type() string { return "target" }

// SetQuery params, override args.
func (t *Target) SetQuery(values url.Values) error {
	// TODO: check mutability

	for key, vals := range values {
		for _, attr := range t.Kwargs.Values {
			if attr.Name != key {
				continue
			}

			switch attr.KindType {
			case KindType{Kind: KindString}:
				if len(vals) > 1 {
					return fmt.Errorf("error: unexpected number of params: %v", vals)
				}
				// TODO: attr validation?
				// TODO: set field on kwargs.
				return fmt.Errorf("url query not implemented: %q", attr)
			default:
				return fmt.Errorf("url query type not supported: %q", attr)
			}

		}
	}
	return nil
}
