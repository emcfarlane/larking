package starlarkstruct

import (
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

type Module = starlarkstruct.Module

// MakeModule may be used as the implementation of a Starlark built-in
// function, module(name, **kwargs). It returns a new module with the
// specified name and members.
func MakeModule(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackPositionalArgs(b.Name(), args, nil, 1, &name); err != nil {
		return nil, err
	}
	members := make(starlark.StringDict, len(kwargs))
	for _, kwarg := range kwargs {
		k := string(kwarg[0].(starlark.String))
		members[k] = kwarg[1]
	}
	return &Module{Name: name, Members: members}, nil
}
