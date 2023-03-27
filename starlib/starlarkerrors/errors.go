// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
// Package errors implements functions to manipulate errors.
package starlarkerrors

import (
	"errors"
	"fmt"
	"regexp"
	"sort"

	"go.starlark.net/starlark"
	"larking.io/starlib/starext"
	"larking.io/starlib/starlarkstruct"
)

func NewModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "errors",
		Members: starlark.StringDict{
			"error": starext.MakeBuiltin("errors.error", MakeError),
			"catch": starext.MakeBuiltin("errors.catch", MakeCatch),
		},
	}
}

type Error struct {
	err error
}

func NewError(err error) Error {
	return Error{err: err}
}

func MakeError(thread *starlark.Thread, _ string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	failFn, ok := starlark.Universe["fail"].(*starlark.Builtin)
	if !ok {
		return nil, fmt.Errorf("internal builtin fail not found")
	}
	if _, err := starlark.Call(thread, failFn, args, kwargs); err != nil {
		// Error on purpose, capture error printing.
		return Error{err: err}, nil
	}
	return nil, fmt.Errorf("fail failed to error")
}

func (e Error) Err() error            { return e.err }
func (e Error) Error() string         { return e.err.Error() }
func (e Error) String() string        { return fmt.Sprintf("<error %q>", e.err.Error()) }
func (e Error) Type() string          { return "errors.error" }
func (e Error) Freeze()               {} // immutable
func (e Error) Truth() starlark.Bool  { return starlark.Bool(e.err != nil) }
func (e Error) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: %s", e.Type()) }

type errorAttr func(e Error) starlark.Value

var errorAttrs = map[string]errorAttr{
	"matches": func(e Error) starlark.Value { return starext.MakeMethod(e, "matches", e.matches) },
	"kind":    func(e Error) starlark.Value { return starext.MakeMethod(e, "kind", e.kind) },
}

func (e Error) Attr(name string) (starlark.Value, error) {
	if a := errorAttrs[name]; a != nil {
		return a(e), nil
	}
	return nil, nil
}
func (e Error) AttrNames() []string {
	names := make([]string, 0, len(errorAttrs))
	for name := range errorAttrs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (e Error) matches(_ *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var pattern string
	if err := starlark.UnpackArgs(fnname, args, kwargs, "pattern", &pattern); err != nil {
		return nil, err
	}
	ok, err := regexp.MatchString(pattern, e.err.Error())
	if err != nil {
		return nil, fmt.Errorf("error.matches: %s", err)
	}
	return starlark.Bool(ok), nil
}

// errorKind as "is" is a reserved keyword.
func (e Error) kind(_ *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var err Error
	if err := starlark.UnpackArgs(fnname, args, kwargs, "err", &err); err != nil {
		return nil, err
	}
	ok := errors.Is(e.err, err.err)
	return starlark.Bool(ok), nil
}

type Result struct {
	value starlark.Value
	isErr bool
}

func (v Result) String() string        { return fmt.Sprintf("<result %q>", v.value.String()) }
func (v Result) Type() string          { return "errors.result" }
func (v Result) Freeze()               { v.value.Freeze() }
func (v Result) Truth() starlark.Bool  { return starlark.Bool(!v.isErr) }
func (v Result) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: %s", v.Type()) }

// MakeCatch evaluates f() and returns its evaluation error message
// if it failed or the value if it succeeded.
func MakeCatch(thread *starlark.Thread, _ string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) == 0 {
		return Result{value: starlark.None}, nil
	}

	fn, ok := args[0].(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf("errors.call: expected callable got %T", args[0])
	}
	args = args[1:]
	v, err := starlark.Call(thread, fn, args, kwargs)
	if err != nil {
		return Result{
			value: NewError(err),
			isErr: true,
		}, nil
	}
	return Result{value: v}, nil
}

func (v Result) AttrNames() []string { return []string{"err", "val"} }
func (v Result) Attr(name string) (starlark.Value, error) {
	switch name {
	case "val":
		if v.isErr {
			return nil, v.value.(Error)
		}
		return v.value, nil
	case "err":
		if v.isErr {
			return v.value, nil
		}
		return starlark.None, nil
	default:
		return nil, nil
	}
}

// ResultIterator implements starlark.Iterator for Result.
// Allows to destruct Result into value and error.
type ResultIterator struct {
	result Result
	index  int
}

func (i *ResultIterator) Next(p *starlark.Value) bool {
	switch i.index {
	case 0: // value
		if !i.result.isErr {
			*p = i.result.value
		} else {
			*p = starlark.None
		}
	case 1: // error
		if i.result.isErr {
			*p = i.result.value
		} else {
			*p = starlark.None
		}
	default:
		return false
	}
	i.index++
	return true
}
func (i *ResultIterator) Done() {}

func (v Result) Iterate() starlark.Iterator {
	return &ResultIterator{result: v}
}
func (v Result) Len() int { return 2 }
