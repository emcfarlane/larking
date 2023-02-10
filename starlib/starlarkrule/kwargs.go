// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlarkrule

import (
	"fmt"

	"go.starlark.net/starlark"
)

type Kwargs struct {
	Values []AttrNameValue
	Deps   []*Label
}

func NewKwargs(attrs []AttrName, kwargs []starlark.Tuple) (*Kwargs, error) {
	attrSeen := make([]bool, len(attrs))
	attrIndex := make(map[string]int)
	for i, v := range attrs {
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
		attr := attrs[i]
		attrSeen[i] = true

		typeName, err := toType(value)
		if err != nil {
			return nil, err
		}

		if typeName == TypeLabel {
			deps = append(deps, value.(*Label))
		} else if typeName != attr.FullName {
			return nil, fmt.Errorf(
				"invalid type %q for %q", value.Type(), attr.FullName,
			)
		}

		values = append(values, AttrNameValue{
			Attr:  attr.Attr,
			Name:  name,
			Value: value,
		})
	}

	for i, attr := range attrs {
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

		for _, v := range dep.Values {
			fullName, err := toType(v)
			if err != nil {
				return nil, err
			}

			if fullName == attr.FullName {
				return v, nil
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
