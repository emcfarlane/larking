// Copyright 2017 The Bazel Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package starlarkstruct defines a mutable version of the Starlark type
// 'struct', a language extension.
//
package starlarkstruct

import (
	"fmt"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

// Make is the implementation of a built-in function that instantiates
// a mutable struct from the specified keyword arguments.
//
// An application can add 'struct' to the Starlark environment like so:
//
// 	globals := starlark.StringDict{
// 		"struct":  starlark.NewBuiltin("struct", starlarkstruct.Make),
// 	}
//
func Make(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 {
		return nil, fmt.Errorf("struct: unexpected positional arguments")
	}
	return FromKeywords(Default, kwargs), nil
}

// FromKeywords returns a new struct instance whose fields are specified by the
// key/value pairs in kwargs.  (Each kwargs[i][0] must be a starlark.String.)
func FromKeywords(constructor starlark.Value, kwargs []starlark.Tuple) *Struct {
	if constructor == nil {
		panic("nil constructor")
	}
	d := make(starlark.StringDict, len(kwargs))
	for _, kwarg := range kwargs {
		k := string(kwarg[0].(starlark.String))
		v := kwarg[1]
		d[k] = v
	}
	return &Struct{
		constructor: constructor,
		members:     d,
	}
}

// FromStringDict returns a new struct instance whose elements are those of d.
// The constructor parameter specifies the constructor; use Default for an ordinary struct.
func FromStringDict(constructor starlark.Value, d starlark.StringDict) *Struct {
	if constructor == nil {
		panic("nil constructor")
	}
	return &Struct{
		constructor: constructor,
		members:     d,
	}
}

// Struct is a mutable Starlark type that maps field names to values.
// Based on the immutable starlark struct "go.starlark.net/starlarkstruct".
// A frozen Struct is identical to the immutable definition.
type Struct struct {
	constructor starlark.Value
	frozen      bool
	members     starlark.StringDict
}

// Default is the default constructor for structs.
const Default = starlark.String("struct")

var (
	_ starlark.HasAttrs    = (*Struct)(nil)
	_ starlark.HasBinary   = (*Struct)(nil)
	_ starlark.HasSetField = (*Struct)(nil)
)

// ToStringDict adds a name/value entry to d for each field of the struct.
func (s *Struct) ToStringDict(d starlark.StringDict) {
	for key, val := range s.members {
		d[key] = val
	}
}

func (s *Struct) String() string {
	buf := new(strings.Builder)
	if s.constructor == Default {
		// NB: The Java implementation always prints struct
		// even for Bazel provider instances.
		buf.WriteString("struct") // avoid String()'s quotation
	} else {
		buf.WriteString(s.constructor.String())
	}
	buf.WriteByte('(')
	for i, k := range s.AttrNames() {
		v := s.members[k]
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(k)
		buf.WriteString(" = ")
		buf.WriteString(v.String())
	}
	buf.WriteByte(')')
	return buf.String()
}

// Constructor returns the constructor used to create this struct.
func (s *Struct) Constructor() starlark.Value { return s.constructor }

func (s *Struct) Type() string         { return "struct" }
func (s *Struct) Truth() starlark.Bool { return true } // even when empty
func (s *Struct) Hash() (uint32, error) {
	// Same algorithm as starlarkstruct.Struct.
	var x, m uint32 = 8731, 9839
	for _, k := range s.AttrNames() {
		v := s.members[k]
		namehash, _ := starlark.String(k).Hash()
		x = x ^ 3*namehash
		y, err := v.Hash()
		if err != nil {
			return 0, err
		}
		x = x ^ y*m
		m += 7349
	}
	return x, nil
}
func (s *Struct) Freeze() {
	if s.frozen {
		return
	}
	for _, v := range s.members {
		v.Freeze()
	}
	s.frozen = true
}

func (x *Struct) Binary(op syntax.Token, y starlark.Value, side starlark.Side) (starlark.Value, error) {
	if y, ok := y.(*Struct); ok && op == syntax.PLUS {
		if side == starlark.Right {
			x, y = y, x
		}

		if eq, err := starlark.Equal(x.constructor, y.constructor); err != nil {
			return nil, fmt.Errorf("in %s + %s: error comparing constructors: %v",
				x.constructor, y.constructor, err)
		} else if !eq {
			return nil, fmt.Errorf("cannot add structs of different constructors: %s + %s",
				x.constructor, y.constructor)
		}

		z := make(starlark.StringDict, len(x.members)+len(y.members))
		for k, v := range x.members {
			z[k] = v
		}
		for k, v := range y.members {
			z[k] = v
		}
		return FromStringDict(x.constructor, z), nil
	}
	return nil, nil // unhandled
}

// Attr returns the value of the specified field.
func (s *Struct) Attr(name string) (starlark.Value, error) {
	if v, ok := s.members[name]; ok {
		return v, nil
	}
	var ctor string
	if s.constructor != Default {
		ctor = s.constructor.String() + " "
	}
	return nil, starlark.NoSuchAttrError(fmt.Sprintf("%sstruct has no .%s attribute", ctor, name))
}

// AttrNames returns a new sorted list of the struct members.
func (s *Struct) AttrNames() []string { return s.members.Keys() }

// SetField sets a fields value.
func (s *Struct) SetField(name string, value starlark.Value) error {
	if s.frozen {
		return fmt.Errorf("cannot insert into frozen struct")
	}
	if _, ok := s.members[name]; !ok {
		return fmt.Errorf("invalid field name for struct")
	}
	s.members[name] = value
	return nil
}

func structsEqual(x, y *Struct, depth int) (bool, error) {
	if len(x.members) != len(y.members) {
		return false, nil
	}

	if eq, err := starlark.Equal(x.constructor, y.constructor); err != nil {
		return false, fmt.Errorf("error comparing struct constructors %v and %v: %v",
			x.constructor, y.constructor, err)
	} else if !eq {
		return false, nil
	}

	for k, xv := range x.members {
		yv, ok := y.members[k]
		if !ok {
			return false, nil
		}

		if eq, err := starlark.EqualDepth(xv, yv, depth-1); err != nil {
			return false, err
		} else if !eq {
			return false, nil
		}
	}
	return true, nil
}

func (x *Struct) CompareSameType(op syntax.Token, y_ starlark.Value, depth int) (bool, error) {
	switch y := y_.(*Struct); op {
	case syntax.EQL:
		return structsEqual(x, y, depth)
	case syntax.NEQ:
		eq, err := structsEqual(x, y, depth)
		return !eq, err
	default:
		return false, fmt.Errorf("%s %s %s not implemented", x.Type(), op, y.Type())
	}
}
