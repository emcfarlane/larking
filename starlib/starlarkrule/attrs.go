// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlarkrule

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/emcfarlane/larking/starlib/starext"
	"github.com/emcfarlane/larking/starlib/starlarkstruct"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

func NewAttrModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "attr",
		Members: starlark.StringDict{
			"bool":     starext.MakeBuiltin("attr.bool", attrBool),
			"int":      starext.MakeBuiltin("attr.int", attrInt),
			"int_list": starext.MakeBuiltin("attr.int_list", attrIntList),
			"label":    starext.MakeBuiltin("attr.label", attrLabel),
			//TODO:"label_keyed_string_dict": starext.MakeBuiltin("attr.label_keyed_string_dict", attrLabelKeyedStringDict),
			"label_list": starext.MakeBuiltin("attr.label_list", attrLabelList),
			//"output":      starext.MakeBuiltin("attr.output", attrOutput),
			//"output_list": starext.MakeBuiltin("attr.output_list", attrOutputList),
			"string": starext.MakeBuiltin("attr.string", attrString),
			//TODO:"string_dict":             starext.MakeBuiltin("attr.string_dict", attrStringDict),
			"string_list": starext.MakeBuiltin("attr.string_list", attrStringList),
			//TODO:"string_list_dict":        starext.MakeBuiltin("attr.string_list_dict", attrStringListDict),
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

	AttrArgsConstructor starlark.String = "attr.args" // starlarkstruct constructor
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

	//Output bool // declare as output value
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
	var x, mult uint32 = 0x345678, 1000003
	for _, elem := range []starlark.Value{
		starlark.String(a.Typ),
		starlark.String(a.Def.String()), // TODO
		starlark.String(a.Doc),
		starlark.Bool(a.Executable),
		starlark.Bool(a.Mandatory),
		starlark.Bool(a.AllowEmpty),
		starlark.String(fmt.Sprintf("%v", a.AllowFiles)),
		starlark.String(fmt.Sprintf("%v", a.Values)),
	} {
		y, err := elem.Hash()
		if err != nil {
			return 0, err
		}
		x = x ^ y*mult
		mult += 82520
	}
	return x, nil
}

func (a *Attr) IsValidType(value starlark.Value) bool {
	var ok bool
	switch a.Typ {
	case AttrTypeBool:
		_, ok = (value).(starlark.Bool)
	case AttrTypeInt:
		_, ok = (value).(starlark.Int)
	case AttrTypeIntList:
		_, ok = (value).(*starlark.List)
	case AttrTypeLabel:
		_, ok = (value).(*Label)
		fmt.Printf("label: %T\n", value)
	//case attrTypeLabelKeyedStringDict:
	case AttrTypeLabelList:
		list, lok := (value).(*starlark.List)
		if !lok {
			return false
		}
		for i, n := 0, list.Len(); i < n; i++ {
			_, ok = (value).(*Label)
			if !ok {
				break
			}
		}
	case AttrTypeOutput:
		_, ok = (value).(starlark.String)
	case AttrTypeOutputList:
		_, ok = (value).(*starlark.List)
	case AttrTypeString:
		_, ok = (value).(starlark.String)
	//case attrTypeStringDict:
	case AttrTypeStringList:
		_, ok = (value).(*starlark.List)
	//case attrTypeStringListDict:
	default:
		panic(fmt.Sprintf("unhandled type: %s", a.Typ))
	}
	return ok
}

func attrEqual(x, y *Attr, depth int) (bool, error) {
	if ok := (x.Typ == y.Typ &&
		x.Def == y.Def &&
		x.Executable == y.Executable &&
		x.Mandatory == y.Mandatory &&
		x.AllowEmpty == y.AllowEmpty); !ok {
		return ok, nil
	}

	if ok, err := starlark.EqualDepth(x.Def, y.Def, depth-1); !ok || err != nil {
		return ok, err
	}

	switch x := x.Values.(type) {
	case []string:
		y, ok := y.Values.([]string)
		if !ok {
			return false, nil
		}
		if len(x) != len(y) {
			return false, nil
		}
		for i, n := 0, len(x); i < n; i++ {
			if x[i] != y[i] {
				return false, nil
			}
		}

	case []int:
		y, ok := y.Values.([]int)
		if !ok {
			return false, nil
		}
		if len(x) != len(y) {
			return false, nil
		}
		for i, n := 0, len(x); i < n; i++ {
			if x[i] != y[i] {
				return false, nil
			}
		}
	case nil:
		if ok := x == y.Values; !ok {
			return ok, nil
		}
	default:
		return false, nil
	}
	return true, nil
}

func (x *Attr) CompareSameType(op syntax.Token, y_ starlark.Value, depth int) (bool, error) {
	y := y_.(*Attr)
	switch op {
	case syntax.EQL:
		return attrEqual(x, y, depth)
	case syntax.NEQ:
		eq, err := attrEqual(x, y, depth)
		return !eq, err
	default:
		return false, fmt.Errorf("%s %s %s not implemented", x.Type(), op, y.Type())
	}
}

// Attribute attr.bool(default=False, doc='', mandatory=False)
func attrBool(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		def       bool
		doc       string
		mandatory bool
	)

	if err := starlark.UnpackArgs(
		fnname, args, kwargs,
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
func attrInt(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		def       = starlark.MakeInt(0)
		doc       string
		mandatory bool
		values    *starlark.List
	)

	if err := starlark.UnpackArgs(
		fnname, args, kwargs,
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
func attrIntList(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		def        *starlark.List
		doc        string
		mandatory  bool
		allowEmpty bool
	)
	if err := starlark.UnpackArgs(
		fnname, args, kwargs,
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
func attrLabel(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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
		fnname, args, kwargs,
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

	var defValue starlark.Value = starlark.None
	if len(def) > 0 {
		defValue = def
	}

	return &Attr{
		Typ:        AttrTypeLabel,
		Def:        defValue,
		Doc:        doc,
		Mandatory:  mandatory,
		Values:     vals,
		AllowFiles: af,
	}, nil
}

// attr.label_list(allow_empty=True, *, default=[], doc='', allow_files=None, providers=[], flags=[], mandatory=False, cfg=None, aspects=[])
func attrLabelList(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		def        *starlark.List
		doc        string
		mandatory  bool
		allowEmpty bool = true
		allowFiles starlark.Value
	)
	if err := starlark.UnpackArgs(
		fnname, args, kwargs,
		"default?", &def, "doc?", &doc, "mandatory?", &mandatory, "allow_empty?", &allowEmpty, "allow_files?", &allowFiles,
	); err != nil {
		return nil, err
	}

	var defValue starlark.Value = starlark.None
	if def != nil {
		iter := def.Iterate()
		var x starlark.Value
		for iter.Next(&x) {
			if _, ok := starlark.AsString(x); !ok {
				return nil, fmt.Errorf("got %s, want string", x.Type())
			}
		}
		iter.Done()
		defValue = def
	}

	af, err := parseAllowFiles(allowFiles)
	if err != nil {
		return nil, err
	}

	return &Attr{
		Typ:        AttrTypeLabelList,
		Doc:        doc,
		Def:        defValue,
		Mandatory:  mandatory,
		AllowEmpty: allowEmpty,
		AllowFiles: af,
	}, nil
}

// Attribute attr.output(doc='', mandatory=False)
func attrOutput(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		doc       string
		mandatory bool
	)

	if err := starlark.UnpackArgs(
		fnname, args, kwargs,
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
func attrOutputList(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		doc        string
		mandatory  bool
		allowEmpty bool
	)
	if err := starlark.UnpackArgs(
		fnname, args, kwargs,
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

func attrString(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		def       starlark.String
		doc       string
		mandatory bool
		values    *starlark.List
	)

	if err := starlark.UnpackArgs(
		fnname, args, kwargs,
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

func attrStringList(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		def        *starlark.List
		doc        string
		mandatory  bool
		allowEmpty bool
	)
	if err := starlark.UnpackArgs(
		fnname, args, kwargs,
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

// Attrs -> AttrArgs
type Attrs struct {
	osd    starext.OrderedStringDict
	frozen bool
}

func (a *Attrs) String() string {
	buf := new(strings.Builder)
	buf.WriteString("attrs")
	buf.WriteByte('(')
	for i := 0; i < a.osd.Len(); i++ {
		k, v := a.osd.KeyIndex(i)
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(k)
		buf.WriteString(" = ")
		buf.WriteString(v.String())
	}
	buf.WriteByte(')')
	return buf.String()
}
func (a *Attrs) Truth() starlark.Bool { return true } // even when empty
func (a *Attrs) Type() string         { return "attrs" }
func (a *Attrs) Hash() (uint32, error) {
	// Same algorithm as struct...
	var x, m uint32 = 8731, 9839
	for i, n := 0, a.osd.Len(); i < n; i++ {
		k, v := a.osd.KeyIndex(i)
		namehash, _ := starlark.String(k).Hash()
		x = x ^ 3*namehash
		y, err := v.Hash()
		if err != nil {
			return 0, err
		}
		x = x ^ y*m
		m += 7349
	}
	return x, nil
}
func (a *Attrs) Freeze() {
	if a.frozen {
		return
	}
	a.frozen = true
	for i, n := 0, a.osd.Len(); i < n; i++ {
		a.osd.Index(i).Freeze()
	}
}

// checkMutable reports an error if the list should not be mutated.
// verb+" list" should describe the operation.
func (a *Attrs) checkMutable(verb string) error {
	if a.frozen {
		return fmt.Errorf("cannot %s frozen attrs", verb)
	}
	return nil
}

func (a *Attrs) Attr(name string) (starlark.Value, error) {
	if v, ok := a.osd.Get(name); ok {
		return v, nil
	}
	return nil, starlark.NoSuchAttrError(
		fmt.Sprintf("attrs has no .%s attribute", name))
}
func (a *Attrs) AttrNames() []string { return a.osd.Keys() }

func attrsEqual(x, y *Attrs, depth int) (bool, error) {
	if x.Len() != y.Len() {
		return false, nil
	}
	for i, n := 0, x.Len(); i < n; i++ {
		x, y := x.Index(i).(*Attr), y.Index(i).(*Attr)
		eq, err := attrEqual(x, y, depth-1)
		if !eq || err != nil {
			return eq, err
		}
	}
	return true, nil
}

func (x *Attrs) CompareSameType(op syntax.Token, y_ starlark.Value, depth int) (bool, error) {
	y := y_.(*Attrs)
	switch op {
	case syntax.EQL:
		return attrsEqual(x, y, depth)
	case syntax.NEQ:
		eq, err := attrsEqual(x, y, depth)
		return !eq, err
	default:
		return false, fmt.Errorf("%s %s %s not implemented", x.Type(), op, y.Type())
	}
}
func (a *Attrs) Index(i int) starlark.Value { return a.osd.Index(i) }
func (a *Attrs) Len() int                   { return a.osd.Len() }

func MakeAttrs(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs(fnname, args, nil); err != nil {
		return nil, err
	}

	osd := starext.NewOrderedStringDict(len(kwargs))
	for _, kwarg := range kwargs {
		name, _ := starlark.AsString(kwarg[0])
		a, ok := kwarg[1].(*Attr)
		if !ok {
			return nil, fmt.Errorf("unexpected attribute value type: %T", kwarg[1])
		}
		osd.Insert(name, a)
	}
	osd.Sort()

	return &Attrs{
		osd:    *osd,
		frozen: false,
	}, nil
}

func (a *Attrs) Get(name string) (*Attr, bool) {
	attr, ok := a.osd.Get(name)
	if !ok {
		return nil, ok
	}
	return attr.(*Attr), ok
}

func (a *Attrs) MakeArgs(source *Label, kwargs []starlark.Tuple) (*AttrArgs, error) {
	attrSeen := make(map[string]bool)
	attrArgs := starext.NewOrderedStringDict(len(kwargs))
	for _, kwarg := range kwargs {
		name := string(kwarg[0].(starlark.String))
		value := kwarg[1]

		attr, ok := a.Get(name)
		if !ok {
			return nil, fmt.Errorf("unexpected attribute: %s", name)
		}

		value, err := asAttrValue(source, name, attr, value)
		if err != nil {
			return nil, err
		}
		attrArgs.Insert(name, value)
		attrSeen[name] = true
	}

	// Mandatory checks
	for i, n := 0, a.osd.Len(); i < n; i++ {
		name, x := a.osd.KeyIndex(i)
		attr := x.(*Attr)
		if !attrSeen[name] {
			if attr.Mandatory {
				return nil, fmt.Errorf("missing mandatory attribute: %s", name)
			}
			attrArgs.Insert(name, attr.Def)
		}
	}
	attrArgs.Sort()
	s := starlarkstruct.OrderedStringDictAsStruct(AttrArgsConstructor, attrArgs)
	return &AttrArgs{
		attrs:  a,
		Struct: *s,
	}, nil
}

func (a *Attrs) Name() string { return "attrargs" }
func (a *Attrs) CallInternal(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs("attrs", args, nil); err != nil {
		return nil, err
	}
	source, err := ParseLabel(thread.Name)
	if err != nil {
		fmt.Println("Here?", err)
		return nil, err
	}
	return a.MakeArgs(source, kwargs)
}

func (a *Attrs) checkArgs(thread *starlark.Thread, value starlark.Value) (*AttrArgs, error) {
	s, ok := value.(*AttrArgs)
	if !ok {
		return nil, fmt.Errorf("expected %s, got %v", AttrArgsConstructor, value.Type())
	}

	attrSeen := make(map[string]bool)
	names := s.AttrNames()

	for _, name := range names {
		value, err := s.Attr(name)
		if value == nil || err != nil {
			return nil, err
		}

		x, ok := a.osd.Get(name)
		if !ok {
			return nil, fmt.Errorf("unexpected attribute: %s", name)
		}
		attr := x.(*Attr)
		if ok := attr.IsValidType(value); !ok {
			return nil, fmt.Errorf("invalid attr %v: %v", attr, value)
		}
		attrSeen[name] = true
	}

	// Mandatory checks
	for i, n := 0, a.osd.Len(); i < n; i++ {
		name, x := a.osd.KeyIndex(i)
		attr := x.(*Attr)
		if !attrSeen[name] {
			if attr.Mandatory {
				return nil, fmt.Errorf("missing mandatory attribute: %s", name)
			}
		}
	}
	return s, nil
}

func asAttrValue(
	source *Label,
	name string,
	attr *Attr,
	value starlark.Value,
) (starlark.Value, error) {
	errField := func(msg string) error {
		return fmt.Errorf(
			"%s %s(%s): %v", msg, name, attr.Typ, value,
		)
	}
	errError := func(err error) error {
		return fmt.Errorf(
			"invalid %s(%s): %v: %v",
			name, attr.Typ, value, err,
		)
	}
	toLabel := func(v starlark.Value) (*Label, error) {
		switch v := (v).(type) {
		case starlark.String:
			l, err := source.Parse(string(v))
			if err != nil {
				return nil, errError(err)
			}
			return l, nil
		case *Label:
			return v, nil
		default:
			return nil, errField("invalid")
		}
	}

	if attr.IsValidType(value) {
		return value, nil
	}

	switch attr.Typ {
	case AttrTypeLabel:
		if value == starlark.None {
			return value, nil
		}

		l, err := toLabel(value)
		if err != nil {
			return nil, err
		}
		return l, nil
	//case attrTypeLabelKeyedStringDict:
	case AttrTypeLabelList:
		if value == starlark.None {
			return value, nil
		}

		list, ok := (value).(*starlark.List)
		if !ok {
			return nil, errField("type")
		}

		var elems []starlark.Value
		for i, n := 0, list.Len(); i < n; i++ {
			val := list.Index(i)

			l, err := toLabel(val)
			if err != nil {
				return nil, err
			}
			elems = append(elems, l)
		}
		return starlark.NewList(elems), nil
	default:
		return nil, errField("undefined")
	}
}

type AttrArgs struct {
	attrs *Attrs
	starlarkstruct.Struct
}

func attrArgsEqual(x, y *AttrArgs, depth int) (bool, error) {
	if ok, err := attrsEqual(x.attrs, y.attrs, depth-1); !ok || err != nil {
		return ok, err
	}
	return x.Struct.CompareSameType(syntax.EQL, &y.Struct, depth-1)
}

func (x *AttrArgs) CompareSameType(op syntax.Token, y_ starlark.Value, depth int) (bool, error) {
	y := y_.(*AttrArgs)
	switch op {
	case syntax.EQL:
		return attrArgsEqual(x, y, depth)
	case syntax.NEQ:
		eq, err := attrArgsEqual(x, y, depth)
		return !eq, err
	default:
		return false, fmt.Errorf("%s %s %s not implemented", x.Type(), op, y.Type())
	}
}

func (a *AttrArgs) Attrs() *Attrs { return a.attrs }

func (a *AttrArgs) Clone() *AttrArgs {
	osd := starext.NewOrderedStringDict(a.attrs.Len())
	a.ToOrderedStringDict(osd)
	s := starlarkstruct.OrderedStringDictAsStruct(AttrArgsConstructor, osd)
	return &AttrArgs{
		attrs:  a.attrs,
		Struct: *s,
	}
}

var _ (starlark.Callable) = (*Attrs)(nil)

//go:embed info.star
var infoSrc string

var (
	DefaultInfo   *Attrs
	ContainerInfo *Attrs
)

func init() {
	info, err := starlark.ExecFile(
		&starlark.Thread{Name: "internal"},
		"rule/info.star",
		infoSrc,
		starlark.StringDict{
			"attr":  NewAttrModule(),
			"attrs": starext.MakeBuiltin("rule.attrs", MakeAttrs),
		},
	)
	if err != nil {
		panic(err)
	}
	DefaultInfo = info["DefaultInfo"].(*Attrs)
	ContainerInfo = info["ContainerInfo"].(*Attrs)
}
