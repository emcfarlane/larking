// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlarkerrors

import (
	"errors"
	"fmt"
	"regexp"
	"sort"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

func NewModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "errors",
		Members: starlark.StringDict{
			"new":  starlark.NewBuiltin("errors.new", MakeError),
			"call": starlark.NewBuiltin("errors.call", MakeCall),
		},
	}
}

type Error struct {
	err error
}

func NewError(err error) Error {
	return Error{err: err}
}

func MakeError(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	failFn, ok := starlark.Universe["fail"].(*starlark.Builtin)
	if !ok {
		return nil, fmt.Errorf("internal builtin fail not found")
	}
	_, err := failFn.CallInternal(thread, args, kwargs)
	if err == nil {
		return nil, fmt.Errorf("fail failed to error")
	}
	return Error{err: err}, nil
}

func (e Error) Err() error            { return e.err }
func (e Error) Error() string         { return e.err.Error() }
func (e Error) String() string        { return fmt.Sprintf("<error %q>", e.err.Error()) }
func (e Error) Type() string          { return "errors.error" }
func (e Error) Freeze()               {} // immutable
func (e Error) Truth() starlark.Bool  { return starlark.Bool(e.err != nil) }
func (e Error) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: %s", e.Type()) }

var errorMethods = map[string]*starlark.Builtin{
	"matches": starlark.NewBuiltin("errors.error.matches", errorMatches),
	"kind":    starlark.NewBuiltin("errors.error.kind", errorKind),
}

func (e Error) Attr(name string) (starlark.Value, error) {
	b := errorMethods[name]
	if b == nil {
		return nil, nil
	}
	return b.BindReceiver(e), nil
}
func (e Error) AttrNames() []string {
	names := make([]string, 0, len(errorMethods))
	for name := range errorMethods {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func errorMatches(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	e := b.Receiver().(Error)

	var pattern string
	if err := starlark.UnpackArgs("error.matches", args, kwargs, "pattern", &pattern); err != nil {
		return nil, err
	}
	ok, err := regexp.MatchString(pattern, e.err.Error())
	if err != nil {
		return nil, fmt.Errorf("error.matches: %s", err)
	}
	return starlark.Bool(ok), nil
}

// errorKind as "is" is a reserved keyword.
func errorKind(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	e := b.Receiver().(Error)

	var err Error
	if err := starlark.UnpackArgs("error.is", args, kwargs, "err", &err); err != nil {
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
func (v Result) Freeze()               {} // immutable
func (v Result) Truth() starlark.Bool  { return starlark.Bool(!v.isErr) }
func (v Result) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: %s", v.Type()) }

// MakeCall evaluates f() and returns its evaluation error message
// if it failed or the value if it succeeded.
func MakeCall(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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
