package starext

import (
	"fmt"

	"go.starlark.net/starlark"
)

type BuiltinFn func(*starlark.Thread, string, starlark.Tuple, []starlark.Tuple) (starlark.Value, error)

type Builtin struct {
	name string
	fn   BuiltinFn
}

func MakeBuiltin(name string, fn BuiltinFn) Builtin {
	return Builtin{
		name: name,
		fn:   fn,
	}
}

func (b Builtin) Name() string          { return b.name }
func (b Builtin) Freeze()               {} // immutable
func (b Builtin) Hash() (uint32, error) { return starlark.String(b.name).Hash() }
func (b Builtin) String() string {
	return fmt.Sprintf("<builtin_function %s>", b.Name())
}
func (b Builtin) Type() string         { return "builtin_function" }
func (b Builtin) Truth() starlark.Bool { return true }
func (b Builtin) CallInternal(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return b.fn(thread, b.name, args, kwargs)
}

type Method struct {
	Builtin
	recv starlark.Value
}

func MakeMethod(recv starlark.Value, name string, fn BuiltinFn) Method {
	return Method{
		Builtin: MakeBuiltin(name, fn),
		recv:    recv,
	}
}

func (m Method) String() string {
	return fmt.Sprintf("<builtin_method %s of %s value>", m.name, m.recv.Type())
}
func (m Method) Type() string { return "builtin_method" }
func (m Method) CallInternal(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return m.fn(thread, m.recv.Type()+"."+m.name, args, kwargs)
}
