// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlarkrule

import (
	"fmt"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

func NewAttrModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "attr",
		Members: starlark.StringDict{
			"bool":     starlark.NewBuiltin("attr.bool", attrBool),
			"int":      starlark.NewBuiltin("attr.int", attrInt),
			"int_list": starlark.NewBuiltin("attr.int_list", attrIntList),
			"label":    starlark.NewBuiltin("attr.label", attrLabel),
			//TODO:"label_keyed_string_dict": starlark.NewBuiltin("attr.label_keyed_string_dict", attrLabelKeyedStringDict),
			"label_list":  starlark.NewBuiltin("attr.label_list", attrLabelList),
			"output":      starlark.NewBuiltin("attr.output", attrOutput),
			"output_list": starlark.NewBuiltin("attr.output_list", attrOutputList),
			"string":      starlark.NewBuiltin("attr.string", attrString),
			//TODO:"string_dict":             starlark.NewBuiltin("attr.string_dict", attrStringDict),
			"string_list": starlark.NewBuiltin("attr.string_list", attrStringList),
			//TODO:"string_list_dict":        starlark.NewBuiltin("attr.string_list_dict", attrStringListDict),
		},
	}
}

type AttrType string

const (
	AttrTypeBool                 AttrType = "attr.bool"
	AttrTypeInt                  AttrType = "attr.int"
	AttrTypeIntList              AttrType = "attr.int_list"
	AttrTypeLabel                AttrType = "attr.label"
	AttrTypeLabelKeyedStringDict AttrType = "attr.label_keyed_string_dict"
	AttrTypeLabelList            AttrType = "attr.label_list"
	AttrTypeOutput               AttrType = "attr.output"
	AttrTypeOutputList           AttrType = "attr.output_list"
	AttrTypeString               AttrType = "attr.string"
	AttrTypeStringDict           AttrType = "attr.string_dict"
	AttrTypeStringList           AttrType = "attr.string_list"
	AttrTypeStringListDict       AttrType = "attr.string_list_dict"
	AttrTypeResolver             AttrType = "attr.resolver"

	Attrs starlark.String = "attrs" // starlarkstruct constructor
)

// Attr defines attributes to a rules attributes.
type Attr struct {
	Typ        AttrType
	Def        starlark.Value // default
	Doc        string
	Executable bool
	Mandatory  bool
	AllowEmpty bool
	AllowFiles allowedFiles // nil, bool, globlist([]string)
	Values     interface{}  // []typ

	// TODO: resolver
	//Resolver starlark.Callable
	//Ins Outs Attr
}

func (a *Attr) String() string {
	var b strings.Builder
	b.WriteString(string(a.Typ))
	b.WriteString("(")
	b.WriteString("default = ")
	b.WriteString(a.Def.String())
	b.WriteString(", doc = " + a.Doc)
	b.WriteString(", executable = ")
	b.WriteString(starlark.Bool(a.Executable).String())
	b.WriteString(", mandatory = ")
	b.WriteString(starlark.Bool(a.Mandatory).String())
	b.WriteString(", allow_empty = ")
	b.WriteString(starlark.Bool(a.AllowEmpty).String())
	b.WriteString(", allow_files = ")
	b.WriteString(starlark.Bool(a.AllowFiles.allow).String())

	b.WriteString(", values = ")
	switch v := a.Values.(type) {
	case []string, []int:
		b.WriteString(fmt.Sprintf("%v", v))
	case nil:
		b.WriteString(starlark.None.String())
	default:
		panic(fmt.Sprintf("unhandled values type: %T", a.Values))
	}
	b.WriteString(")")
	return b.String()
}
func (a *Attr) Type() string         { return string(a.Typ) }
func (a *Attr) AttrType() AttrType   { return a.Typ }
func (a *Attr) Freeze()              {} // immutable
func (a *Attr) Truth() starlark.Bool { return true }
func (a *Attr) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: %s", a.Type())
}

// Attribute attr.bool(default=False, doc='', mandatory=False)
func attrBool(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		def       bool
		doc       string
		mandatory bool
	)

	if err := starlark.UnpackArgs(
		"attr.bool", args, kwargs,
		"default?", &def, "doc?", &doc, "mandatory?", &mandatory,
	); err != nil {
		return nil, err
	}

	return &Attr{
		Typ:       AttrTypeBool,
		Def:       starlark.Bool(def),
		Doc:       doc,
		Mandatory: mandatory,
	}, nil
}

// Attribute attr.int(default=0, doc='', mandatory=False, values=[])
func attrInt(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		def       = starlark.MakeInt(0)
		doc       string
		mandatory bool
		values    *starlark.List
	)

	if err := starlark.UnpackArgs(
		"attr.bool", args, kwargs,
		"default?", &def, "doc?", &doc, "mandatory?", &mandatory, "values?", &values,
	); err != nil {
		return nil, err
	}

	var ints []int
	if values != nil {
		iter := values.Iterate()
		var x starlark.Value
		for iter.Next(&x) {
			i, err := starlark.AsInt32(x)
			if err != nil {
				return nil, err
			}
			ints = append(ints, i)
		}
		iter.Done()
	}

	return &Attr{
		Typ:       AttrTypeInt,
		Def:       def,
		Doc:       doc,
		Mandatory: mandatory,
		Values:    ints,
	}, nil
}

// Attribute attr.int_list(mandatory=False, allow_empty=True, *, default=[], doc='')
func attrIntList(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		def        *starlark.List
		doc        string
		mandatory  bool
		allowEmpty bool
	)
	if err := starlark.UnpackArgs(
		"attr.bool", args, kwargs,
		"default?", &def, "doc?", &doc, "mandatory?", &mandatory, "allowEmpty?", &allowEmpty,
	); err != nil {
		return nil, err
	}

	iter := def.Iterate()
	var x starlark.Value
	for iter.Next(&x) {
		if _, err := starlark.AsInt32(x); err != nil {
			return nil, err
		}
	}
	iter.Done()

	return &Attr{
		Typ:        AttrTypeIntList,
		Def:        def,
		Doc:        doc,
		Mandatory:  mandatory,
		AllowEmpty: allowEmpty,
	}, nil
}

type allowedFiles struct {
	allow bool
	types []string
}

func parseAllowFiles(allowFiles starlark.Value) (allowedFiles, error) {
	switch v := allowFiles.(type) {
	case nil:
		return allowedFiles{allow: false}, nil
	case starlark.Bool:
		return allowedFiles{allow: bool(v)}, nil
	default:
		panic(fmt.Sprintf("TODO: handle allow_files type: %T", allowFiles))
	}
}

// Attribute attr.label(default=None, doc='', executable=False, allow_files=None, allow_single_file=None, mandatory=False, providers=[], allow_rules=None, cfg=None, aspects=[])
func attrLabel(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		def        starlark.String
		doc        string
		executable = false
		mandatory  bool
		values     *starlark.List
		allowFiles starlark.Value

		// TODO: more types!
		//providers
	)

	if err := starlark.UnpackArgs(
		"attr.bool", args, kwargs,
		"default?", &def, "doc?", &doc, "executable", &executable, "mandatory?", &mandatory, "values?", &values, "allow_files?", &allowFiles,
	); err != nil {
		return nil, err
	}

	var vals []string
	if values != nil {
		iter := values.Iterate()
		var x starlark.Value
		for iter.Next(&x) {
			s, ok := starlark.AsString(x)
			if !ok {
				return nil, fmt.Errorf("got %s, want string", x.Type())
			}
			vals = append(vals, s)
		}
		iter.Done()
	}

	af, err := parseAllowFiles(allowFiles)
	if err != nil {
		return nil, err
	}

	return &Attr{
		Typ:        AttrTypeLabel,
		Def:        def,
		Doc:        doc,
		Mandatory:  mandatory,
		Values:     vals,
		AllowFiles: af,
	}, nil
}

// attr.label_list(allow_empty=True, *, default=[], doc='', allow_files=None, providers=[], flags=[], mandatory=False, cfg=None, aspects=[])
func attrLabelList(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		def        *starlark.List
		doc        string
		mandatory  bool
		allowEmpty bool = true
		allowFiles starlark.Value
	)
	if err := starlark.UnpackArgs(
		"attr.bool", args, kwargs,
		"default?", &def, "doc?", &doc, "mandatory?", &mandatory, "allow_empty?", &allowEmpty, "allow_files?", &allowFiles,
	); err != nil {
		return nil, err
	}

	// TODO: default checks?
	if def != nil {
		iter := def.Iterate()
		var x starlark.Value
		for iter.Next(&x) {
			if _, ok := starlark.AsString(x); !ok {
				return nil, fmt.Errorf("got %s, want string", x.Type())
			}
		}
		iter.Done()
	}

	af, err := parseAllowFiles(allowFiles)
	if err != nil {
		return nil, err
	}

	return &Attr{
		Typ:        AttrTypeLabelList,
		Def:        def,
		Doc:        doc,
		Mandatory:  mandatory,
		AllowEmpty: allowEmpty,
		AllowFiles: af,
	}, nil
}

// Attribute attr.output(doc='', mandatory=False)
func attrOutput(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		doc       string
		mandatory bool
	)

	if err := starlark.UnpackArgs(
		"attr.bool", args, kwargs,
		"doc?", &doc, "mandatory?", &mandatory,
	); err != nil {
		return nil, err
	}

	return &Attr{
		Typ:       AttrTypeOutput,
		Doc:       doc,
		Mandatory: mandatory,
	}, nil
}

// Attribute attr.output_list(allow_empty=True, *, doc='', mandatory=False)
func attrOutputList(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		doc        string
		mandatory  bool
		allowEmpty bool
	)
	if err := starlark.UnpackArgs(
		"attr.bool", args, kwargs,
		"doc?", &doc, "mandatory?", &mandatory, "allowEmpty?", &allowEmpty,
	); err != nil {
		return nil, err
	}

	return &Attr{
		Typ:        AttrTypeOutputList,
		Doc:        doc,
		Mandatory:  mandatory,
		AllowEmpty: allowEmpty,
	}, nil
}

func attrString(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		def       starlark.String
		doc       string
		mandatory bool
		values    *starlark.List
	)

	if err := starlark.UnpackArgs(
		"attr.bool", args, kwargs,
		"default?", &def, "doc?", &doc, "mandatory?", &mandatory, "values?", &values,
	); err != nil {
		return nil, err
	}

	var strings []string
	if values != nil {
		iter := values.Iterate()
		var x starlark.Value
		for iter.Next(&x) {
			s, ok := starlark.AsString(x)
			if !ok {
				return nil, fmt.Errorf("got %s, want string", x.Type())
			}
			strings = append(strings, s)
		}
		iter.Done()
	}

	return &Attr{
		Typ:       AttrTypeString,
		Def:       def,
		Doc:       doc,
		Mandatory: mandatory,
		Values:    strings,
	}, nil
}

func attrStringList(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		def        *starlark.List
		doc        string
		mandatory  bool
		allowEmpty bool
	)
	if err := starlark.UnpackArgs(
		"attr.bool", args, kwargs,
		"default?", &def, "doc?", &doc, "mandatory?", &mandatory, "allowEmpty?", &allowEmpty,
	); err != nil {
		return nil, err
	}

	// Check defaults are all strings
	if def != nil {
		iter := def.Iterate()
		var x starlark.Value
		for iter.Next(&x) {
			if _, ok := starlark.AsString(x); !ok {
				return nil, fmt.Errorf("got %s, want string", x.Type())
			}
		}
		iter.Done()
	}

	return &Attr{
		Typ:       AttrTypeStringList,
		Def:       def,
		Doc:       doc,
		Mandatory: mandatory,
	}, nil
}
