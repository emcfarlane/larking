// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package starlarkio implements readers and writers.
package starlarkio

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"go.starlark.net/starlark"
	"larking.io/starlib/starext"
)

func ToReader(v starlark.Value) (io.Reader, error) {
	switch x := v.(type) {
	case starlark.String:
		return strings.NewReader(string(x)), nil
	case starlark.Bytes:
		return strings.NewReader(string(x)), nil
	case *Reader:
		return x.Reader, nil
	case starext.Value:
		if v, ok := x.Reflect().Interface().(io.Reader); ok {
			return v, nil
		}
		return nil, fmt.Errorf("invalid reader type: %q", v.Type())
	case io.Reader:
		return x, nil
	default:
		return nil, fmt.Errorf("invalid reader type: %q", v.Type())
	}
}

type Reader struct {
	io.Reader
	frozen bool
}

func (v *Reader) String() string        { return "<reader>" }
func (v *Reader) Type() string          { return "io.reader" }
func (v *Reader) Freeze()               { v.frozen = true }
func (v *Reader) Truth() starlark.Bool  { return starlark.Bool(v.Reader != nil) }
func (v *Reader) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: %s", v.Type()) }

type readerAttr func(e *Reader) starlark.Value

// TODO: optional methods io.Closer, etc.
var readerAttrs = map[string]readerAttr{
	"read_all": func(r *Reader) starlark.Value { return starext.MakeMethod(r, "read_all", r.readAll) },
	//"read":     func(r *Reader) starlark.Value { return starext.MakeMethod(r, "read", r.read) },
}

func (v *Reader) Attr(name string) (starlark.Value, error) {
	if a := readerAttrs[name]; a != nil {
		return a(v), nil
	}
	return nil, nil
}
func (v *Reader) AttrNames() []string {
	names := make([]string, 0, len(readerAttrs))
	for name := range readerAttrs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (v *Reader) readAll(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 0); err != nil {
		return nil, err
	}

	x, err := io.ReadAll(v.Reader)
	if err != nil {
		return nil, err
	}
	return starlark.Bytes(x), nil
}
