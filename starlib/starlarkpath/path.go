package starlarkpath

import (
	"fmt"
	"path"

	"github.com/emcfarlane/larking/starlib/starext"
	"github.com/emcfarlane/larking/starlib/starlarkstruct"
	"go.starlark.net/starlark"
)

func NewModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "errors",
		Members: starlark.StringDict{
			"base":   starext.MakeBuiltin("path.base", Base),
			"clean":  starext.MakeBuiltin("path.clean", Clean),
			"dir":    starext.MakeBuiltin("path.dir", Dir),
			"ext":    starext.MakeBuiltin("path.ext", Ext),
			"is_abs": starext.MakeBuiltin("path.is_abs", IsAbs),
			"join":   starext.MakeBuiltin("path.join", Join),
			"match":  starext.MakeBuiltin("path.match", Match),
			"split":  starext.MakeBuiltin("path.split", Split),
		},
	}
}

func Base(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var base string
	if err := starlark.UnpackArgs(fnname, args, kwargs, "base", &base); err != nil {
		return nil, err
	}
	return starlark.String(path.Base(base)), nil
}
func Clean(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var clean string
	if err := starlark.UnpackArgs(fnname, args, kwargs, "clean", &clean); err != nil {
		return nil, err
	}
	return starlark.String(path.Clean(clean)), nil
}
func Dir(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var dir string
	if err := starlark.UnpackArgs(fnname, args, kwargs, "dir", &dir); err != nil {
		return nil, err
	}
	return starlark.String(path.Dir(dir)), nil
}
func Ext(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var ext string
	if err := starlark.UnpackArgs(fnname, args, kwargs, "ext", &ext); err != nil {
		return nil, err
	}
	return starlark.String(path.Ext(ext)), nil
}
func IsAbs(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs(fnname, args, kwargs, "path", &name); err != nil {
		return nil, err
	}
	return starlark.Bool(path.IsAbs(name)), nil
}
func Join(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	n := len(args)
	strs := make([]string, n)
	for i := 0; i < n; i++ {
		arg := args.Index(i)
		str, ok := starlark.AsString(arg)
		if !ok {
			return nil, fmt.Errorf("expected string, got %v", arg.Type())
		}
		strs = append(strs, str)
	}
	return starlark.String(path.Join(strs...)), nil
}
func Match(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var pattern, name string
	if err := starlark.UnpackArgs(fnname, args, kwargs, "pattern", &pattern, "name", &name); err != nil {
		return nil, err
	}
	ok, err := path.Match(pattern, name)
	return starlark.Bool(ok), err
}
func Split(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs(fnname, args, kwargs, "path", &name); err != nil {
		return nil, err
	}
	dir, file := path.Split(name)
	return starlark.Tuple{starlark.String(dir), starlark.String(file)}, nil
}
