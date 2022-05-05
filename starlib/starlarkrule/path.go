package starlarkrule

import (
	"fmt"
	"path"

	"github.com/emcfarlane/larking/starlib/starext"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

var pathModule = &starlarkstruct.Module{
	Name: "path",
	Members: starlark.StringDict{
		"join": starext.MakeBuiltin("path.join", pathJoin),
	},
}

func pathJoin(_ *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackPositionalArgs(
		fnname, nil, kwargs, 0,
	); err != nil {
		return nil, err
	}
	vals := make([]string, len(args))
	for i, arg := range args {
		var ok bool
		vals[i], ok = starlark.AsString(arg)
		if !ok {
			return nil, fmt.Errorf("unexpected type: %s", arg.Type())
		}
	}
	return starlark.String(path.Join(vals...)), nil
}
