package starlarkrule

import (
	"larking.io/starlib/starext"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// actions are embedded rule implementations.

var actionsModule = &starlarkstruct.Module{
	Name: "actions",
	Members: starlark.StringDict{
		"archive":   archiveModule,
		"container": containerModule,
		"label":     starext.MakeBuiltin("label", MakeLabel),
		"run":       starext.MakeBuiltin("run", run),
	},
}

func Actions() *starlarkstruct.Module { return actionsModule }
