// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlarkrule

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
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
			"rule":       starext.MakeBuiltin("rule.rule", MakeRule),
			"attr":       NewAttrModule(),
			"provider":   starext.MakeBuiltin("rule.provider", MakeProvider),
			"attributes": starext.MakeBuiltin("rule.attrs", MakeProvider),
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

// ParseLabel creates a new Label relative to the source.
func ParseLabel(source, label string) (*Label, error) {
	u, err := url.Parse(source)
	if err != nil {
		return nil, fmt.Errorf("invalid source: %v", err)
	}

	q := u.Query()
	key := q.Get("key")
	dir, _ := path.Split(key)
	// TODO: validate key?

	l, err := url.Parse(label)
	if err != nil {
		return nil, fmt.Errorf("invalid label: %v", err)
	}

	// If empty scheme take path as key.
	if l.Scheme == "" {
		key = path.Join(dir, l.Path)
		q.Set("key", key)
		if rq := l.RawQuery; rq != "" {
			q.Set("keyargs", rq)
		}
		u.RawQuery = q.Encode()
	} else {
		u = l // use l as an absolute URL.
	}
	return &Label{*u, false}, nil
}

func MakeLabel(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackPositionalArgs(
		fnname, args, kwargs, 1, &name,
	); err != nil {
		return nil, err
	}
	return ParseLabel(thread.Name, name)
}

func AsLabel(v starlark.Value) (*Label, error) {
	l, ok := v.(*Label)
	if !ok {
		return nil, fmt.Errorf("expected label, got %s", v.Type())
	}
	return l, nil
}

type Rule struct {
	impl *starlark.Function // implementation function
	ins  map[string]*Attr   // input attribute types
	outs map[string]*Attr   // output attribute types

	frozen bool
}

func dictToAttrs(attrs *starlark.Dict) (map[string]*Attr, error) {
	m := make(map[string]*Attr)
	for _, item := range attrs.Items() {
		name := string(item[0].(starlark.String))
		a, ok := item[1].(*Attr)
		if !ok {
			return nil, fmt.Errorf("unexpected attribute value type: %T", item[1])
		}
		m[name] = a
	}
	if _, ok := m["name"]; ok {
		return nil, fmt.Errorf("invalid attr: \"name\", cannot be specified")
	}
	return m, nil
}

// MakeRule creates a new rule instance. Accepts the following optional kwargs:
// "impl", "ins" and "outs".
func MakeRule(_ *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		impl = new(starlark.Function)
		ins  = new(starlark.Dict)
		outs = new(starlark.Dict)
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
func (r *Rule) Ins() AttrFields          { return AttrFields{r.ins} }
func (r *Rule) Outs() AttrFields         { return AttrFields{r.outs} }

type Builder interface {
	RegisterTarget(thread *starlark.Thread, target *Target) error
}

const bldKey = "builder"

func SetBuilder(thread *starlark.Thread, builder Builder) {
	thread.SetLocal(bldKey, builder)
}
func GetBuilder(thread *starlark.Thread) (Builder, error) {
	if bld, ok := thread.Local(bldKey).(Builder); ok {
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

	// Call from go(...)
	if len(args) > 0 {
		return nil, fmt.Errorf("error: got %d arguments, want 0", len(args))
	}

	attrSeen := make(map[string]bool)
	attrArgs := starext.NewOrderedStringDict(len(kwargs))
	for _, kwarg := range kwargs {
		name := string(kwarg[0].(starlark.String))
		value := kwarg[1]
		//value.Freeze()? Needed?
		fmt.Println("\t\tname:", name)

		attr, ok := r.ins[name]
		if !ok {
			return nil, fmt.Errorf("unexpected attribute: %s", name)
		}

		if err := asAttrValue(thread, name, attr, &value); err != nil {
			return nil, err
		}

		fmt.Println("setting value", name, value)
		attrArgs.Insert(name, value)
		attrSeen[name] = true
	}

	// Mandatory checks
	for name, a := range r.ins {
		if !attrSeen[name] {
			if a.Mandatory {
				return nil, fmt.Errorf("missing mandatory attribute: %s", name)
			}
			attrArgs.Insert(name, a.Def)
		}
	}

	//module, ok := thread.Local("module").(string)
	//if !ok {
	//	return nil, fmt.Errorf("error internal: unknown module")
	//}
	attrArgs.Sort()

	target, err := NewTarget(thread, r, attrArgs)
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
	url    url.URL
	rule   *Rule
	args   starext.OrderedStringDict // attribute args
	frozen bool
}

func NewTarget(
	thread *starlark.Thread,
	rule *Rule,
	args *starext.OrderedStringDict,
) (*Target, error) {
	// Assert name exists.
	const field = "name"
	nv, ok := args.Get(field)
	if !ok {
		return nil, fmt.Errorf("missing required field %q", field)
	}
	name, ok := starlark.AsString(nv)
	if !ok || !isStringAlphabetic(name) {
		return nil, fmt.Errorf("invalid field %q: %q", field, name)
	}

	l, err := ParseLabel(thread.Name, name)
	if err != nil {
		return nil, err
	}

	return &Target{
		url:  l.URL,
		rule: rule,
		args: *args,
	}, nil
}

func (t *Target) Clone() *Target {
	args := starext.NewOrderedStringDict(t.args.Len())
	for i, n := 0, t.args.Len(); i < n; i++ {
		key, val := t.args.KeyIndex(i)
		args.Insert(key, val)
	}
	return &Target{
		url:  t.url,  // copied
		rule: t.rule, // immuatble
		args: *args,  // cloned
	}
}

// TODO?
func (t *Target) Hash() (uint32, error) { return 0, nil }

func (t *Target) String() string {
	return t.url.String()
}

func (t *Target) FullString() string {
	var buf strings.Builder
	buf.WriteString(t.url.String())

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
	return buf.String()
}

// SetQuery params, override args.
func (t *Target) SetQuery(values url.Values) error {
	// TODO: check mutability

	for key, vals := range values {
		attr, ok := t.rule.ins[key]
		if !ok {
			return fmt.Errorf("error: unknown query param: %s", key)
		}

		switch attr.AttrType() {
		case AttrTypeString:
			if len(vals) > 1 {
				return fmt.Errorf("error: unexpected number of params: %v", vals)
			}
			s := vals[0]
			// TODO: attr validation?
			t.args.Insert(key, starlark.String(s))

		default:
			panic("TODO: query parsing")
		}
	}
	return nil
}

func (t *Target) Attrs() *starlarkstruct.Struct {
	return starlarkstruct.FromOrderedStringDict(Attrs, &t.args)
}

func (t *Target) Rule() *Rule { return t.rule }

func (t *Target) Deps() ([]*Label, error) {
	fmt.Println("--- DEPS ---")
	n := t.args.Len()
	deps := make([]*Label, 0, n/2)
	for i := 0; i < n; i++ {
		key, arg := t.args.KeyIndex(i)
		attr := t.rule.ins[key]
		fmt.Println("attrs", "key", key, "arg", arg, "attr", attr)

		switch attr.Typ {
		case AttrTypeLabel:
			l, err := AsLabel(arg)
			if err != nil {
				return nil, err
			}
			fmt.Println("\tlabel")
			deps = append(deps, l)

		case AttrTypeLabelList:
			v := arg.(starlark.Indexable)
			for i, n := 0, v.Len(); i < n; i++ {
				x := v.Index(i)
				l, err := AsLabel(x)
				if err != nil {
					return nil, err
				}
				fmt.Println("\tlabel")
				deps = append(deps, l)
			}

		case AttrTypeLabelKeyedStringDict:
			panic("TODO")
		}
	}
	fmt.Println("-----------")
	return deps, nil
}

type Provider struct {
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

}
