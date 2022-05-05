// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlarkrule

import (
	"fmt"
	"net/url"
	"path"
	"regexp"

	"github.com/emcfarlane/larking/starlib/starext"
	"github.com/emcfarlane/larking/starlib/starlarkstruct"
	"go.starlark.net/starlark"
)

func NewModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "rule",
		Members: starlark.StringDict{
			"rule":    starext.MakeBuiltin("rule", MakeRule),
			"attr":    NewAttrModule(),
			"label":   starext.MakeBuiltin("label", MakeLabel),
			"actions": actionsModule,
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

func ParseLabel(dir, label string) (*Label, error) {
	u, err := url.Parse(label)
	if err != nil {
		return nil, err
	}
	fmt.Printf("%+v\n", *u)
	if err != nil {
		return nil, fmt.Errorf("error: invalid label %s", label)
	}
	if u.Scheme == "" {
		u.Scheme = "file"
		if len(u.Path) > 0 && u.Path[0] != '/' {
			u.Path = path.Join(dir, u.Path)
		}
	}
	if u.Scheme != "file" {
		return nil, fmt.Errorf("error: unknown scheme %s", u.Scheme)
	}

	// HACK: host -> path
	if u.Scheme == "file" && u.Host != "" {
		u.Path = path.Join(u.Host, u.Path)
		u.Host = ""
	}
	return &Label{*u, false}, nil
}

func MakeLabel(_ *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var dir, label string
	if err := starlark.UnpackPositionalArgs(
		fnname, args, kwargs, 2, &dir, &label,
	); err != nil {
		return nil, err
	}
	return ParseLabel(dir, label)
}

func MakeRuleCtx(key, dir, tmpDir string, attrs starlark.StringDict) *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "ctx",
		Members: starlark.StringDict{
			"dir":             starlark.String(dir),
			"tmp_dir":         starlark.String(tmpDir),
			"build_dir":       starlark.String(path.Dir(key)),
			"build_file_path": starlark.String(path.Join(path.Dir(key), "BUILD.star")),

			"key":   starlark.String(key),
			"attrs": starlarkstruct.FromStringDict(Attrs, attrs),
		},
	}
}

// Rule is a laze build rule for implementing actions.
type Rule struct {
	impl  *starlark.Function        // implementation function
	attrs map[string]*Attr          // attribute types
	args  starext.OrderedStringDict // attribute args

	frozen bool
}

func MakeRule(_ *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		impl  = new(starlark.Function)
		attrs = new(starlark.Dict)
	)
	if err := starlark.UnpackArgs(
		fnname, args, kwargs,
		"impl", &impl, "attrs?", &attrs,
	); err != nil {
		return nil, err
	}

	// type checks
	if impl.NumParams() != 1 {
		return nil, fmt.Errorf("unexpected number of params: %d", impl.NumParams())
	}

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
		return nil, fmt.Errorf("name cannot be an attribute")
	}
	m["name"] = &Attr{
		Typ:       AttrTypeString,
		Def:       starlark.String(""),
		Doc:       "Name of rule",
		Mandatory: true,
	}

	return &Rule{
		impl:  impl,
		attrs: m, // key -> type
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

// SetQuery params, override args.
func (r *Rule) SetQuery(values url.Values) error {
	for key, vals := range values {
		attr, ok := r.attrs[key]
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
			r.args.Insert(key, starlark.String(s))

		default:
			panic("TODO: query parsing")
		}
	}
	return nil
}

type AttrArg struct {
	*Attr
	Arg starlark.Value
}

func (r *Rule) Len() int { return r.args.Len() }
func (r *Rule) Index(i int) AttrArg {
	key, arg := r.args.KeyIndex(i)
	attr := r.attrs[key]
	return AttrArg{attr, arg}
}
func (r *Rule) Impl() *starlark.Function { return r.impl }

// genrule(
// 	cmd = "protoc ...",
// 	deps = ["//:label"],
// 	outs = ["//"],
// 	executable = "file",
// )

var isStringAlphabetic = regexp.MustCompile(`^[a-zA-Z0-9_.]*$`).MatchString

func (r *Rule) Name() string { return "rule" }

func (r *Rule) CallInternal(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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

		a, ok := r.attrs[name]
		if !ok {
			return nil, fmt.Errorf("unexpected attribute: %s", name)
		}

		// Type check attributes args.
		switch a.Typ {
		case AttrTypeBool:
			_, ok = value.(starlark.Bool)
		case AttrTypeInt:
			_, ok = value.(starlark.Int)
		case AttrTypeIntList:
			_, ok = value.(*starlark.List)
			// TODO: list check
		case AttrTypeLabel:
			_, ok = value.(starlark.String)
		//case attrTypeLabelKeyedStringDict:
		case AttrTypeLabelList:
			_, ok = value.(*starlark.List)
		case AttrTypeOutput:
			_, ok = value.(starlark.String)
		case AttrTypeOutputList:
			_, ok = value.(*starlark.List)
		case AttrTypeString:
			_, ok = value.(starlark.String)
		//case attrTypeStringDict:
		case AttrTypeStringList:
			_, ok = value.(*starlark.List)
		//case attrTypeStringListDict:

		default:
			panic(fmt.Sprintf("unhandled type: %s", a.Typ))
		}
		if !ok {
			return nil, fmt.Errorf("invalid field %s(%s): %v", name, a.Typ, value)
		}

		fmt.Println("setting value", name, value)
		attrArgs.Insert(name, value)
		attrSeen[name] = true
	}

	// Mandatory checks
	for name, a := range r.attrs {
		if !attrSeen[name] {
			if a.Mandatory {
				return nil, fmt.Errorf("missing mandatory attribute: %s", name)
			}
			attrArgs.Insert(name, a.Def)
		}
	}
	r.args = *attrArgs

	module, ok := thread.Local("module").(string)
	if !ok {
		return nil, fmt.Errorf("error internal: unknown module")
	}

	// name has to exist.
	nv, _ := attrArgs.Get("name")
	name := string(nv.(starlark.String))

	// TODO: name validation?
	if !isStringAlphabetic(name) {
		return nil, fmt.Errorf("error: invalid name: %s", name)
	}

	dir := path.Dir(module)
	key := path.Join(dir, name)

	// Register Rule in the build.
	//if _, ok := r.builder.rulesCache[key]; ok {
	//	return nil, fmt.Errorf("duplicate rule registered: %s", key)
	//}
	//if r.builder.rulesCache == nil {
	//	r.builder.rulesCache = make(map[string]*rule)
	//}
	//r.builder.rulesCache[key] = r

	if err := bld.RegisterRule(key, r); err != nil {
		return nil, err
	}
	return r, nil
}
