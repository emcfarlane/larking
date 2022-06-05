// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package starlarkio implements readers and writers.
package starlarkio

import (
	"fmt"
	"io"
	"io/ioutil"
	"sort"

	"larking.io/starlib/starext"
	"go.starlark.net/starlark"
)

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

// TODO: check args/kwargs length
func (v *Reader) readAll(_ *starlark.Thread, _ string, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	x, err := ioutil.ReadAll(v.Reader)
	if err != nil {
		return nil, err
	}
	return starlark.Bytes(x), nil
}
