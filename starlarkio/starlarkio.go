package starlarkio

import (
	"fmt"
	"io"
	"io/ioutil"
	"sort"

	"go.starlark.net/starlark"
)

type Reader struct {
	io.Reader
	frozen bool
}

func (v *Reader) String() string        { return "<reader>" }
func (v *Reader) Type() string          { return "io.reader" }
func (v *Reader) Freeze()               { v.frozen = true }
func (v *Reader) Truth() starlark.Bool  { return starlark.Bool(v.Reader != nil) }
func (v *Reader) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: %s", v.Type()) }

// TODO: optional methods io.Closer, etc.
var readerMethods = map[string]*starlark.Builtin{
	"read_all": starlark.NewBuiltin("io.reader.read_all", readerReadAll),
}

func (v *Reader) Attr(name string) (starlark.Value, error) {
	b := readerMethods[name]
	if b == nil {
		return nil, nil
	}
	return b.BindReceiver(v), nil
}
func (v *Reader) AttrNames() []string {
	names := make([]string, 0, len(readerMethods))
	for name := range readerMethods {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// TODO: check args/kwargs length
func readerReadAll(_ *starlark.Thread, b *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	v := b.Receiver().(*Reader)
	x, err := ioutil.ReadAll(v.Reader)
	if err != nil {
		return nil, err
	}
	return starlark.Bytes(x), nil
}
