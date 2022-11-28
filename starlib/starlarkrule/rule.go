// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlarkrule

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"sort"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
	"larking.io/starlib/starext"
	"larking.io/starlib/starlarkstruct"
)

func NewModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "rule",
		Members: starlark.StringDict{
			"rule":     starext.MakeBuiltin("rule.rule", MakeRule),
			"attr":     NewAttrModule(),
			"attrs":    starext.MakeBuiltin("rule.attrs", MakeAttrs),
			"provides": starext.MakeBuiltin("rule.provides", MakeProvides),
			"label":    starext.MakeBuiltin("rule.label", MakeLabel),

			//"DefaultInfo":   DefaultInfo,
			//"ContainerInfo": ContainerInfo,
		},
	}
}

// Label is a resource URL.
type Label struct {
	url.URL
	frozen bool
}

func (*Label) Type() string           { return "label" }
func (l *Label) Truth() starlark.Bool { return l != nil }
func (l *Label) Hash() (uint32, error) {
	// Hash simplified from struct hash.
	var x uint32 = 8731
	namehash, _ := starlark.String(l.String()).Hash()
	x = x ^ 3*namehash
	return x, nil
}
func (l *Label) Freeze() { l.frozen = true }

type labelAttr func(l *Label) starlark.Value

var labelAttrs = map[string]labelAttr{
	"scheme":   func(l *Label) starlark.Value { return starlark.String(l.Scheme) },
	"opaque":   func(l *Label) starlark.Value { return starlark.String(l.Opaque) },
	"user":     func(l *Label) starlark.Value { return starlark.String(l.User.String()) },
	"host":     func(l *Label) starlark.Value { return starlark.String(l.Host) },
	"path":     func(l *Label) starlark.Value { return starlark.String(l.Path) },
	"query":    func(l *Label) starlark.Value { return starlark.String(l.RawQuery) },
	"fragment": func(l *Label) starlark.Value { return starlark.String(l.Fragment) },

	// Blob type parameters.
	"bucket": func(l *Label) starlark.Value { return starlark.String(l.BucketURL()) },
	"key":    func(l *Label) starlark.Value { return starlark.String(l.Key()) },
}

func (v *Label) Attr(name string) (starlark.Value, error) {
	if a := labelAttrs[name]; a != nil {
		return a(v), nil
	}
	return nil, nil
}
func (v *Label) AttrNames() []string {
	names := make([]string, 0, len(labelAttrs))
	for name := range labelAttrs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (l *Label) BucketURL() string {
	u := l.URL
	q := u.Query()
	q.Del("key")
	q.Del("keyargs")
	u.RawQuery = q.Encode()
	return u.String()
}
func (l *Label) Key() string { return l.Query().Get("key") }
func (l *Label) KeyArgs() (url.Values, error) {
	s := l.Query().Get("keyargs")
	return url.ParseQuery(s)
}

// Strip keyargs to match target URL.
func (l *Label) CleanURL() string {
	u := l.URL
	q := u.Query()
	q.Del("keyargs")
	u.RawQuery = q.Encode()
	return u.String()
}

func (x *Label) CompareSameType(op syntax.Token, _y starlark.Value, depth int) (bool, error) {
	y := _y.(*Label)
	switch op {
	case syntax.EQL:
		return x.String() == y.String(), nil
	default:
		return false, fmt.Errorf("unsupported comparison: %v", op)
	}

}

// Parse accepts a full formed label or a relative URL.
func (x *Label) Parse(relative string) (*Label, error) {
	u := x.URL // copy

	y, err := url.Parse(relative)
	if err != nil {
		return nil, fmt.Errorf("invalid source: %v", err)
	}
	if y.Scheme != "" {
		return &Label{*y, false}, nil
	}

	// If empty scheme take path as key.
	q := u.Query()
	key := q.Get("key")
	dir, _ := path.Split(key)

	key = path.Join(dir, y.Path)
	q.Set("key", key)
	if rq := y.RawQuery; rq != "" {
		q.Set("keyargs", rq)
	}
	u.RawQuery = q.Encode()

	return &Label{u, false}, nil

}

// ParseLabel creates a new Label relative to the source.
func ParseLabel(label string) (*Label, error) {
	u, err := url.Parse(label)
	if err != nil {
		return nil, fmt.Errorf("invalid label: %v", err)
	}

	// If empty scheme take path as key.
	if u.Scheme == "" {
		return nil, fmt.Errorf("expected absolute URL")
	}
	return &Label{*u, false}, nil
}

func ParseRelativeLabel(source, label string) (*Label, error) {
	l, err := ParseLabel(source)
	if err != nil {
		return nil, err
	}
	return l.Parse(label)
}

func MakeLabel(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackPositionalArgs(
		fnname, args, kwargs, 1, &name,
	); err != nil {
		return nil, err
	}
	return ParseRelativeLabel(thread.Name, name)
}

func AsLabel(v starlark.Value) (*Label, error) {
	l, ok := v.(*Label)
	if !ok {
		return nil, fmt.Errorf("expected label, got %s", v.Type())
	}
	return l, nil
}

type Rule struct {
	impl     *starlark.Function // implementation function
	doc      string
	attrs    *Attrs    // input attribute types
	provides *Provides // output attribute types

	frozen bool
}

// MakeRule creates a new rule instance. Accepts the following optional kwargs:
// "impl", "doc", "attrs" and "provides".
func MakeRule(_ *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		impl     = new(starlark.Function)
		attrs    = new(Attrs)
		provides = new(Provides)
		doc      string
	)
	if err := starlark.UnpackArgs(
		fnname, args, kwargs,
		"impl", &impl,
		"doc?", &doc,
		"attrs?", &attrs,
		"provides?", &provides,
	); err != nil {
		return nil, err
	}

	// Params are now kwargs.
	//// type checks
	//if impl.NumParams() != 1 {
	//	return nil, fmt.Errorf("unexpected number of params: %d", impl.NumParams())
	//}

	if err := attrs.checkMutable("use"); err != nil {
		return nil, err
	}
	//keyName := "name"
	//if _, ok := attrs.osd.Get(keyName); ok {
	//	return nil, fmt.Errorf("reserved %q keyword", keyName)
	//}
	//attrs.osd.Insert(keyName, &Attr{
	//	Typ:       AttrTypeString,
	//	Def:       starlark.String(""),
	//	Doc:       "Name of rule",
	//	Mandatory: true,
	//})

	//pvds := make([]*Attrs, provides.Len())
	//for i, n := 0, provides.Len(); i < n; i++ {
	//	x := provides.Index(i)
	//	attr, ok := x.(*Attrs)
	//	if !ok {
	//		return nil, fmt.Errorf(
	//			"invalid provides[%d] %q, expected %q",
	//			i, x.Type(), (&Attrs{}).Type(),
	//		)
	//	}
	//	pvds[i] = attr
	//}

	// key=dir:target.tar.gz
	// key=dir/target.tar.gz

	return &Rule{
		impl:     impl,
		doc:      doc,
		attrs:    attrs,
		provides: provides,
	}, nil

}

func (r *Rule) String() string       { return "rule()" }
func (r *Rule) Type() string         { return "rule" }
func (r *Rule) Freeze()              { r.frozen = true }
func (r *Rule) Truth() starlark.Bool { return starlark.Bool(!r.frozen) }
func (r *Rule) Hash() (uint32, error) {
	// TODO: can a rule be hashed?
	return 0, fmt.Errorf("unhashable type: rule")
}
func (r *Rule) Impl() *starlark.Function { return r.impl }
func (r *Rule) Attrs() *Attrs            { return r.attrs }
func (r *Rule) Provides() *Provides      { return r.provides }

//*starlark.Set {
//	s := starlark.NewSet(len(r.provides))
//	for _, attrs := range r.provides {
//		if err := s.Insert(attrs); err != nil {
//			panic(err)
//		}
//	}
//	return s
//}

//func (r *Rule) Outs() AttrFields         { return AttrFields{r.outs} }

const bldKey = "builder"

func SetBuilder(thread *starlark.Thread, builder *Builder) {
	thread.SetLocal(bldKey, builder)
}
func GetBuilder(thread *starlark.Thread) (*Builder, error) {
	if bld, ok := thread.Local(bldKey).(*Builder); ok {
		return bld, nil
	}
	return nil, fmt.Errorf("missing builder")
}

// genrule(
// 	cmd = "protoc ...",
// 	deps = ["//:label"],
// 	outs = ["//"],
// 	executable = "file",
// )

var isStringAlphabetic = regexp.MustCompile(`^[a-zA-Z0-9_.]*$`).MatchString

func (r *Rule) Name() string { return "rule" }

func (r *Rule) CallInternal(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	bld, err := GetBuilder(thread)
	if err != nil {
		return nil, err
	}

	if len(args) > 0 {
		return nil, fmt.Errorf("unexpected args")
	}

	source, err := ParseLabel(thread.Name)
	if err != nil {
		return nil, err
	}

	kws, err := NewKwargs(source, r.attrs, kwargs)
	if err != nil {
		return nil, err
	}

	target, err := NewTarget(source, r, kws)
	if err != nil {
		return nil, err
	}

	if err := bld.RegisterTarget(thread, target); err != nil {
		return nil, err
	}
	return r, nil
}
