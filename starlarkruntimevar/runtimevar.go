// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package runtimevar adds configuration variables at runtime.
package starlarkruntimevar

import (
	"fmt"
	"sort"

	"github.com/emcfarlane/larking/starext"
	"github.com/emcfarlane/larking/starlarkthread"
	starlarktime "go.starlark.net/lib/time"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"gocloud.dev/runtimevar"
)

func NewModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "runtimevar",
		Members: starlark.StringDict{
			"open": starext.MakeBuiltin("runtimevar.open", Open),
		},
	}
}

func Open(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 1, &name); err != nil {
		return nil, err
	}

	ctx := starlarkthread.Context(thread)

	variable, err := runtimevar.OpenVariable(ctx, name)
	if err != nil {
		return nil, err
	}

	v := &Variable{
		name:     name,
		variable: variable,
	}
	if err := starlarkthread.AddResource(thread, v); err != nil {
		return nil, err
	}
	return v, nil
}

type Variable struct {
	name     string
	variable *runtimevar.Variable
}

func (v *Variable) String() string        { return fmt.Sprintf("<variable %q>", v.name) }
func (v *Variable) Type() string          { return "runtimevar.variable" }
func (v *Variable) Freeze()               {} // immutable?
func (v *Variable) Truth() starlark.Bool  { return v.variable != nil }
func (v *Variable) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: %s", v.Type()) }
func (v *Variable) Close() error {
	return v.variable.Close()
}

type variableAttr func(v *Variable) starlark.Value

var variableAttrs = map[string]variableAttr{
	"latest": func(v *Variable) starlark.Value { return starext.MakeMethod(v, "latest", v.latest) },
	"close":  func(v *Variable) starlark.Value { return starext.MakeMethod(v, "close", v.close) }, // TODO: expose me?
}

func (v *Variable) Attr(name string) (starlark.Value, error) {
	if a := variableAttrs[name]; a != nil {
		return a(v), nil
	}
	return nil, nil
}
func (v *Variable) AttrNames() []string {
	names := make([]string, 0, len(variableAttrs))
	for name := range variableAttrs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (v *Variable) latest(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 0); err != nil {
		return nil, err
	}

	ctx := starlarkthread.Context(thread)
	snapshot, err := v.variable.Latest(ctx)
	if err != nil {
		return nil, err
	}
	return Snapshot(snapshot), nil
}

func (v *Variable) close(_ *starlark.Thread, fnname string, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if err := v.variable.Close(); err != nil {
		return nil, err
	}
	return starlark.None, nil
}

type Snapshot runtimevar.Snapshot

func (v Snapshot) String() string        { return fmt.Sprintf("<snapshot %v>", v.Value) }
func (v Snapshot) Type() string          { return "runtimevar.snapshot" }
func (v Snapshot) Freeze()               {} // immutable?
func (v Snapshot) Truth() starlark.Bool  { return v.Value != nil }
func (v Snapshot) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: %s", v.Type()) }
func (v Snapshot) AttrNames() []string   { return []string{"value", "update_time"} }
func (v Snapshot) Attr(name string) (starlark.Value, error) {
	switch name {
	case "value":
		switch x := v.Value.(type) {
		case string:
			return starlark.String(x), nil
		case []byte:
			return starlark.Bytes(string(x)), nil
		case map[string]interface{}:
			// TODO: json map
			return nil, fmt.Errorf("TODO: file issue for jsonmap support")
		default:
			return nil, fmt.Errorf("unhandled runtimevar type: %T", x)
		}

	case "update_time":
		return starlarktime.Time(v.UpdateTime), nil
	default:
		return nil, nil
	}

}
