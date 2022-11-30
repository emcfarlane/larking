// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlarkrule

import (
	"fmt"

	"go.starlark.net/starlark"
	"larking.io/starlib/encoding/starlarkproto"
)

type Kwargs struct {
	Values []AttrNameValue
	Deps   []*Label
}

func NewKwargs(attrs *Attrs, kwargs []starlark.Tuple) (*Kwargs, error) {
	attrSeen := make([]bool, len(attrs.nameAttrs))
	attrIndex := make(map[string]int)
	for i, v := range attrs.nameAttrs {
		attrIndex[v.Name] = i
	}

	var (
		values []AttrNameValue
		deps   []*Label
	)
	for _, kwarg := range kwargs {
		name := string(kwarg[0].(starlark.String))
		value := kwarg[1]

		i, ok := attrIndex[name]
		if !ok {
			return nil, fmt.Errorf("unexpected attribute: %s", name)
		}
		attr := attrs.nameAttrs[i]
		attrSeen[i] = true

		errKind := func() error {
			return fmt.Errorf(
				"invalid type %q for %q", value.Type(), attr.Kind,
			)
		}

		switch x := (value).(type) {
		case starlark.Bool:
			if attr.Kind != KindBool {
				return nil, errKind()
			}
		case starlark.Int:
			if attr.Kind != KindInt {
				return nil, errKind()
			}
		case starlark.Float:
			if attr.Kind != KindFloat {
				return nil, errKind()
			}
		case starlark.String:
			if attr.Kind != KindString {
				return nil, errKind()
			}
		case starlark.Bytes:
			if attr.Kind != KindBytes {
				return nil, errKind()
			}
		case *starlark.List:
			if attr.Kind != KindList {
				return nil, errKind()
			}
			errIndexKind := func(i int) error {
				return fmt.Errorf(
					"invalid type %s[%d] %q for %q",
					name, i, value.Type(), attr.Kind,
				)
			}
			for i, n := 0, x.Len(); i < n; i++ {
				v := x.Index(i)
				switch y := (v).(type) {
				case starlark.Bool:
					if attr.ValKind != KindBool {
						return nil, errIndexKind(i)
					}
				case starlark.Int:
					if attr.ValKind != KindInt {
						return nil, errIndexKind(i)
					}
				case starlark.Float:
					if attr.ValKind != KindFloat {
						return nil, errIndexKind(i)
					}
				case starlark.String:
					if attr.ValKind != KindString {
						return nil, errIndexKind(i)
					}
				case starlark.Bytes:
					if attr.ValKind != KindBytes {
						return nil, errIndexKind(i)
					}
				case *starlarkproto.Message:
					if attr.ValKind != KindMessage {
						return nil, errIndexKind(i)
					}
				case *Label:
					deps = append(deps, y)
				default:
					return nil, errIndexKind(i)
				}
			}
		case *starlark.Dict:
			if attr.Kind != KindDict {
				return nil, errKind()
			}
			if attr.Kind != KindList {
				return nil, errKind()
			}
			errKeyKind := func(value starlark.Value) error {
				return fmt.Errorf(
					"dict key type %s[%s] %q for %q",
					name, value.String(), value.Type(), attr.KeyKind,
				)
			}
			errValKind := func(key, value starlark.Value) error {
				return fmt.Errorf(
					"dict value type %s[%s] %s %q for %q",
					name, key.String(), value.String(), value.Type(), attr.ValKind,
				)
			}
			items := x.Items()
			for _, item := range items {
				key := item[0]
				switch y := (key).(type) {
				case starlark.Bool:
					if attr.KeyKind != KindBool {
						return nil, errKeyKind(y)
					}
				case starlark.Int:
					if attr.KeyKind != KindInt {
						return nil, errKeyKind(y)
					}
				case starlark.Float:
					if attr.KeyKind != KindFloat {
						return nil, errKeyKind(y)
					}
				case starlark.String:
					if attr.KeyKind != KindString {
						return nil, errKeyKind(y)
					}
				default:
					return nil, errKeyKind(y)
				}

				val := item[1]
				switch y := (val).(type) {
				case starlark.Bool:
					if attr.KeyKind != KindBool {
						return nil, errValKind(key, y)
					}
				case starlark.Int:
					if attr.KeyKind != KindInt {
						return nil, errValKind(key, y)
					}
				case starlark.Float:
					if attr.KeyKind != KindFloat {
						return nil, errValKind(key, y)
					}
				case starlark.String:
					if attr.KeyKind != KindString {
						return nil, errValKind(key, y)
					}
				case starlark.Bytes:
					if attr.Kind != KindBytes {
						return nil, errValKind(key, y)
					}
				case *starlarkproto.Message:
					if attr.Kind != KindMessage {
						return nil, errValKind(key, y)
					}
				case *Label:
					deps = append(deps, y)
				default:
					return nil, errValKind(key, y)
				}
			}

		case *starlarkproto.Message:
			if attr.Kind != KindMessage {
				return nil, errKind()
			}

		case *Label:
			deps = append(deps, x)
		default:
			return nil, errKind()
		}

		values = append(values, AttrNameValue{
			Attr:  attr.Attr,
			Name:  name,
			Value: value,
		})
	}

	for i, attr := range attrs.nameAttrs {
		if !attrSeen[i] && !attr.Optional {
			return nil, fmt.Errorf("missing mandatory attribute: %q", attr.Name)
		}
	}

	return &Kwargs{
		Values: values,
		Deps:   deps,
	}, nil
}

func (k *Kwargs) Attr(name string) (starlark.Value, error) {
	for _, kwarg := range k.Values {
		if name == kwarg.Name {
			return kwarg.Value, nil
		}
	}
	return nil, nil // no value
}

// Resolve any label deps returning the kwargs for the impl function.
func (k *Kwargs) Resolve(deps []*Action) ([]starlark.Tuple, error) {
	n := len(k.Values)
	kvpairs := make([]starlark.Tuple, 0, n)
	kvpairsAlloc := make(starlark.Tuple, 2*n) // allocate a single backing array

	lookup := make(map[string]*Action, len(deps))
	for _, dep := range deps {
		lookup[dep.Target.Label.String()] = dep
	}

	getValueForAttr := func(attr *Attr, label *Label) (starlark.Value, error) {
		lstr := label.String()
		dep, ok := lookup[lstr]
		if !ok {
			return nil, fmt.Errorf("missing dep: %q", lstr)
		}

		for _, dv := range dep.Values {
			if dv.Attr.KindType == attr.KindType {
				return dv.Value, nil
			}
		}
		if attr.Optional {
			return attr.Default, nil
		}
		return nil, fmt.Errorf("missing mandatory attribute: %q %s", lstr, attr.String())
	}

	for _, x := range k.Values {
		pair := kvpairsAlloc[:2:2]
		kvpairsAlloc = kvpairsAlloc[2:]

		value := x.Value
		switch y := value.(type) {
		case *Label:
			// Resolve label, replace.
			v, err := getValueForAttr(x.Attr, y)
			if err != nil {
				return nil, err
			}
			value = v

		case *starlark.List:
			// resolve any values
			n := y.Len()
			elems := make([]starlark.Value, 0, n)

			for i := 0; i < n; i++ {
				v := y.Index(i)
				l, ok := v.(*Label)
				if !ok {
					elems = append(elems, v)
					continue
				}

				v, err := getValueForAttr(x.Attr, l)
				if err != nil {
					return nil, err
				}
				elems = append(elems, v)
			}

			value = starlark.NewList(elems)

		case *starlark.Dict:
			// resolve any values
			n := y.Len()
			d := starlark.NewDict(n)
			items := y.Items()
			for _, kv := range items {
				key, val := kv[0], kv[1]

				l, ok := val.(*Label)
				if !ok {
					if err := d.SetKey(key, val); err != nil {
						return nil, err
					}
					continue
				}

				v, err := getValueForAttr(x.Attr, l)
				if err != nil {
					return nil, err
				}
				if err := d.SetKey(key, v); err != nil {
					return nil, err
				}
			}

			value = d
		}

		pair[0] = starlark.String(x.Name)
		pair[1] = value
		kvpairs = append(kvpairs, pair)
	}
	return kvpairs, nil
}

func (k *Kwargs) Clone() *Kwargs {

	values := make([]AttrNameValue, len(k.Values))
	copy(values, k.Values)
	return &Kwargs{
		Values: values,
		Deps:   k.Deps, // immutable?
	}
}
