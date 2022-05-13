package starlarkrule

import (
	"github.com/emcfarlane/larking/starlib/starext"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// actions are embedded rule implementations.
var actionsModule = &starlarkstruct.Module{
	Name: "actions",
	Members: starlark.StringDict{
		"files":     starlark.None, //newFilesModule(a),
		"packaging": packagingModule,
		"container": containerModule,
		"run":       starext.MakeBuiltin("run", run),
	},
}
