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

	"github.com/emcfarlane/larking/starlib/starext"
	"github.com/emcfarlane/larking/starlib/starlarkstruct"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

func NewModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "rule",
		Members: starlark.StringDict{
			"rule":        starext.MakeBuiltin("rule.rule", MakeRule),
			"attr":        NewAttrModule(),
			"attrs":       starext.MakeBuiltin("rule.attrs", MakeAttrs),
			"DefaultInfo": DefaultInfo,
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
	impl  *starlark.Function // implementation function
	attrs *Attrs             // input attribute types
	doc   string
	//outs map[string]*Attr   // output attribute types
	provides []*Attrs

	frozen bool
}

// MakeRule creates a new rule instance. Accepts the following optional kwargs:
// "impl", "attrs" and "provides".
func MakeRule(_ *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		impl     = new(starlark.Function)
		attrs    = new(Attrs)
		doc      string
		provides = new(starlark.List)
		//ins  = new(starlark.Dict)
		//outs = new(starlark.Dict)
	)
	if err := starlark.UnpackArgs(
		fnname, args, kwargs,
		"impl", &impl,
		"attrs?", &attrs,
		"doc?", &doc,
		"provides?", &provides,
	); err != nil {
		return nil, err
	}

	// type checks
	if impl.NumParams() != 1 {
		return nil, fmt.Errorf("unexpected number of params: %d", impl.NumParams())
	}

	if err := attrs.checkMutable("use"); err != nil {
		return nil, err
	}
	keyName := "name"
	if _, ok := attrs.osd.Get(keyName); ok {
		return nil, fmt.Errorf("reserved %q keyword", keyName)
	}
	attrs.osd.Insert(keyName, &Attr{
		Typ:       AttrTypeString,
		Def:       starlark.String(""),
		Doc:       "Name of rule",
		Mandatory: true,
	})

	pvds := make([]*Attrs, provides.Len())
	for i, n := 0, provides.Len(); i < n; i++ {
		x := provides.Index(i)
		attr, ok := x.(*Attrs)
		if !ok {
			return nil, fmt.Errorf(
				"invalid provides[%d] %q, expected %q",
				i, x.Type(), (&Attrs{}).Type(),
			)
		}
		pvds[i] = attr
	}

	// key=dir:target.tar.gz
	// key=dir/target.tar.gz

	return &Rule{
		impl:     impl,
		doc:      doc,
		attrs:    attrs,
		provides: pvds,
		//ins:  inAttrs,
		//outs: outAttrs,
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
func (r *Rule) Provides() *starlark.Set {
	s := starlark.NewSet(len(r.provides))
	for _, attrs := range r.provides {
		if err := s.Insert(attrs); err != nil {
			panic(err)
		}
	}
	return s
}

//func (r *Rule) Outs() AttrFields         { return AttrFields{r.outs} }

const bldKey = "builder"

func setBuilder(thread *starlark.Thread, builder *Builder) {
	thread.SetLocal(bldKey, builder)
}
func getBuilder(thread *starlark.Thread) (*Builder, error) {
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
	bld, err := getBuilder(thread)
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

	attrArgs, err := r.attrs.MakeArgs(source, kwargs)
	if err != nil {
		return nil, err
	}

	// Call from go(...)
	//if len(args) > 0 {
	//	return nil, fmt.Errorf("error: got %d arguments, want 0", len(args))
	//}

	//attrSeen := make(map[string]bool)
	//attrArgs := starext.NewOrderedStringDict(len(kwargs))
	//for _, kwarg := range kwargs {
	//	name := string(kwarg[0].(starlark.String))
	//	value := kwarg[1]
	//	//value.Freeze()? Needed?
	//	fmt.Println("\t\tname:", name)

	//	attr, ok := r.ins[name]
	//	if !ok {
	//		return nil, fmt.Errorf("unexpected attribute: %s", name)
	//	}

	//	if err := asAttrValue(thread, name, attr, &value); err != nil {
	//		return nil, err
	//	}

	//	fmt.Println("setting value", name, value)
	//	attrArgs.Insert(name, value)
	//	attrSeen[name] = true
	//}

	//// Mandatory checks
	//for name, a := range r.ins {
	//	if !attrSeen[name] {
	//		if a.Mandatory {
	//			return nil, fmt.Errorf("missing mandatory attribute: %s", name)
	//		}
	//		attrArgs.Insert(name, a.Def)
	//	}
	//}

	//module, ok := thread.Local("module").(string)
	//if !ok {
	//	return nil, fmt.Errorf("error internal: unknown module")
	//}
	//attrArgs.Sort()

	target, err := NewTarget(source, r, attrArgs)
	if err != nil {
		return nil, err
	}

	if err := bld.RegisterTarget(thread, target); err != nil {
		return nil, err
	}
	return r, nil
}

// Target is defined by a call to a rule.
type Target struct {
	label  *Label
	rule   *Rule
	args   AttrArgs //starext.OrderedStringDict // attribute args
	frozen bool
}

func NewTarget(
	source *Label,
	rule *Rule,
	args *AttrArgs, //*starext.OrderedStringDict,
) (*Target, error) {
	// Assert name exists.
	const field = "name"
	nv, err := args.Attr(field)
	if err != nil {
		return nil, fmt.Errorf("missing required field %q", field)
	}
	name, ok := starlark.AsString(nv)
	if !ok || !isStringAlphabetic(name) {
		return nil, fmt.Errorf("invalid field %q: %q", field, name)
	}

	l, err := source.Parse(name)
	if err != nil {
		return nil, err
	}

	return &Target{
		label: l,
		rule:  rule,
		args:  *args,
	}, nil
}

func (t *Target) Clone() *Target {
	return &Target{
		label: t.label,         // immutable?
		rule:  t.rule,          // immuatble
		args:  *t.args.Clone(), // cloned
	}
}

// TODO?
func (t *Target) Hash() (uint32, error) { return 0, nil }

func (t *Target) String() string {
	var buf strings.Builder
	buf.WriteString(t.Type())
	buf.WriteRune('(')

	buf.WriteString(t.label.String())
	for i, n := 0, t.args.Len(); i < n; i++ {
		if i == 0 {
			buf.WriteRune('?')
		} else {
			buf.WriteRune('&')
		}

		key, val := t.args.KeyIndex(i)
		buf.WriteString(key) // already escaped
		buf.WriteRune('=')
		buf.WriteString(url.QueryEscape(val.String()))
	}

	buf.WriteRune(')')
	return buf.String()
}
func (t *Target) Type() string { return "target" }

// SetQuery params, override args.
func (t *Target) SetQuery(values url.Values) error {
	// TODO: check mutability

	for key, vals := range values {
		x, ok := t.rule.attrs.osd.Get(key)
		if !ok {
			return fmt.Errorf("error: unknown query param: %s", key)
		}
		attr := x.(*Attr)

		switch attr.AttrType() {
		case AttrTypeString:
			if len(vals) > 1 {
				return fmt.Errorf("error: unexpected number of params: %v", vals)
			}
			s := vals[0]
			// TODO: attr validation?
			t.args.SetField(key, starlark.String(s))

		default:
			panic("TODO: query parsing")
		}
	}
	return nil
}

func (t *Target) Args() *AttrArgs { return &t.args }
func (t *Target) Rule() *Rule     { return t.rule }

/*type Provider struct {
	// TODO
}

func MakeProvider(_ *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		attrs = new(starlark.Dict)
	)
	if err := starlark.UnpackArgs(
		fnname, args, kwargs,
		"impl", &impl, "ins?", &ins, "outs?", &outs,
	); err != nil {
		return nil, err
	}

	// type checks
	if impl.NumParams() != 1 {
		return nil, fmt.Errorf("unexpected number of params: %d", impl.NumParams())
	}

	inAttrs, err := dictToAttrs(ins)
	if err != nil {
		return nil, err
	}
	inAttrs["name"] = &Attr{
		Typ:       AttrTypeString,
		Def:       starlark.String(""),
		Doc:       "Name of rule",
		Mandatory: true,
	}

	outAttrs, err := dictToAttrs(outs)
	if err != nil {
		return nil, err
	}

	// key=dir:target.tar.gz
	// key=dir/target.tar.gz

	return &Rule{
		impl: impl,
		ins:  inAttrs,
		outs: outAttrs,
	}, nil

}*/
