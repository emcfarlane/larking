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
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
	"google.golang.org/protobuf/reflect/protoreflect"
	"larking.io/starlib/starext"
	"larking.io/starlib/starlarkstruct"
)

func NewModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "rule",
		Members: starlark.StringDict{
			"rule":  starext.MakeBuiltin("rule.rule", MakeRule),
			"attr":  NewAttrModule(),
			"label": starext.MakeBuiltin("rule.label", MakeLabel),
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
	impl        starlark.Callable
	doc         string
	attrs       []AttrName                      // input attribute types
	attrsMap    map[string]AttrName             // lookup attributes
	provides    []*Attr                         // output attribute types
	providesMap map[protoreflect.FullName]*Attr // lookup provides

	frozen bool
}

var reIdentifier = regexp.MustCompile(`^[^\d\W]\w*$`)

// MakeRule creates a new rule instance. Accepts the following optional kwargs:
// "impl", "doc", "attrs" and "provides".
func MakeRule(_ *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		impl = new(starlark.Function)
		//attrs    = new(Attrs)
		//provides = new(Provides)
		attrs    = new(starlark.Dict)
		provides = new(starlark.List)
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

	items := attrs.Items()
	attrNames := make([]AttrName, len(items)+1)
	attrNames[0] = nameAttr
	attrsMap := make(map[string]AttrName)

	for i, item := range items {
		key, val := item[0], item[1]

		name, ok := starlark.AsString(key)
		if !ok || name == "" {
			return nil, fmt.Errorf("unexpected attribute name: %q", name)
		}
		if name == "name" {
			return nil, fmt.Errorf("reserved %q keyword", "name")
		}
		if !reIdentifier.MatchString(name) {
			return nil, fmt.Errorf("invalid name: %q", name)
		}

		a, ok := val.(*Attr)
		if !ok {
			return nil, fmt.Errorf("unexpected attribute value type: %q: %T", name, val)
		}

		attrName := AttrName{
			Attr: a,
			Name: name,
		}
		attrNames[i+1] = attrName
		attrsMap[name] = attrName
	}

	// Params are now kwargs.
	//// type checks
	//if impl.NumParams() != 1 {
	//	return nil, fmt.Errorf("unexpected number of params: %d", impl.NumParams())
	//}

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

	pvds := make([]*Attr, provides.Len())
	pvdsMap := make(map[protoreflect.FullName]*Attr)
	for i, n := 0, provides.Len(); i < n; i++ {
		x := provides.Index(i)
		attr, ok := x.(*Attr)
		if !ok {
			return nil, fmt.Errorf(
				"invalid provides[%d] %q, expected %q",
				i, x.Type(), (&Attr{}).Type(),
			)
		}
		pvds[i] = attr
		pvdsMap[attr.FullName] = attr
	}

	// key=dir:target.tar.gz
	// key=dir/target.tar.gz

	return &Rule{
		impl:        impl,
		doc:         doc,
		attrs:       attrNames,
		attrsMap:    attrsMap,
		provides:    pvds,
		providesMap: pvdsMap,
	}, nil

}

func (r *Rule) String() string {
	buf := new(strings.Builder)
	buf.WriteString("rule")
	buf.WriteByte('(')

	buf.WriteString("impl = ")
	buf.WriteString(r.impl.String())

	buf.WriteString(", doc = ")
	buf.WriteString(r.doc)

	{
		buf.WriteString("attrs")
		buf.WriteByte('{')
		for i, attrName := range r.attrs {
			if i > 0 {
				buf.WriteString(", ")
			}
			buf.WriteString(attrName.Name)
			buf.WriteString(attrName.String())
		}
		buf.WriteByte('}')
	}

	{
		buf.WriteString("provides")
		buf.WriteByte('[')
		for i, attr := range r.provides {
			if i > 0 {
				buf.WriteString(", ")
			}
			buf.WriteString(attr.String())
		}
		buf.WriteByte(']')
	}
	buf.WriteByte(')')
	return buf.String()
}
func (r *Rule) Type() string         { return "rule" }
func (r *Rule) Freeze()              { r.frozen = true }
func (r *Rule) Truth() starlark.Bool { return starlark.Bool(!r.frozen) }
func (r *Rule) Hash() (uint32, error) {
	// TODO: can a rule be hashed?
	return 0, fmt.Errorf("unhashable type: rule")
}
func (r *Rule) Impl() starlark.Callable { return r.impl }
func (r *Rule) Attrs() []AttrName       { return r.attrs }
func (r *Rule) Provides() []*Attr       { return r.provides }

const bldKey = "builder"

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

	kws, err := NewKwargs(r.attrs, kwargs)
	if err != nil {
		return nil, err
	}

	source, err := ParseLabel(thread.Name)
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
