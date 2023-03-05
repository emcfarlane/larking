package starlarkio

import (
	"fmt"
	"io"
	"sort"

	"go.starlark.net/starlark"
	"larking.io/starlib/starext"
)

func ToWriter(v starlark.Value) (io.Writer, error) {
	switch x := v.(type) {
	case *Writer:
		return x.Writer, nil
	case io.Writer:
		return x, nil
	case starext.Value:
		if v, ok := x.Reflect().Interface().(io.Writer); ok {
			return v, nil
		}
		return nil, fmt.Errorf("invalid reader type: %q", v.Type())
	default:
		return nil, fmt.Errorf("invalid writer type: %q", v.Type())
	}
}

type Writer struct {
	io.Writer
	frozen bool
}

func (v *Writer) String() string        { return "<reader>" }
func (v *Writer) Type() string          { return "io.reader" }
func (v *Writer) Freeze()               { v.frozen = true }
func (v *Writer) Truth() starlark.Bool  { return starlark.Bool(v.Writer != nil) }
func (v *Writer) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: %s", v.Type()) }

type writerAttr func(e *Writer) starlark.Value

// TODO: optional methods io.Closer, etc.
var writerAttrs = map[string]writerAttr{
	// "write": func(w *Writer) starlark.Value { return starext.MakeMethod(w, "write", w.write) },
}

func (v *Writer) Attr(name string) (starlark.Value, error) {
	if a := writerAttrs[name]; a != nil {
		return a(v), nil
	}
	return nil, nil
}
func (v *Writer) AttrNames() []string {
	names := make([]string, 0, len(readerAttrs))
	for name := range readerAttrs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

//// TODO: check args/kwargs length
//func (v *Writer) write(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
//	var p starlark.Bytes
//	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 1, &p); err != nil {
//		return nil, err
//	}
//
//	x, err := v.Writer.Write(p)
//	if err != nil {
//		return nil, err
//	}
//	return starlark.MakeInt64(x), nil
//}
