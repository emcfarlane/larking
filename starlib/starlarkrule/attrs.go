// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlarkrule

import (
	_ "embed"
	"fmt"
	"strings"

	"go.starlark.net/starlark"
	"larking.io/starlib/encoding/starlarkproto"
	"larking.io/starlib/starext"
	"larking.io/starlib/starlarkstruct"
)

func NewAttrModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "attr",
		Members: starlark.StringDict{
			"bool":    starext.MakeBuiltin("attr.bool", attrBool),
			"int":     starext.MakeBuiltin("attr.int", attrInt),
			"float":   starext.MakeBuiltin("attr.float", attrFloat),
			"string":  starext.MakeBuiltin("attr.string", attrString),
			"bytes":   starext.MakeBuiltin("attr.string", attrBytes),
			"list":    starext.MakeBuiltin("attr.string", attrList),
			"dict":    starext.MakeBuiltin("attr.string", attrDict),
			"message": starext.MakeBuiltin("attr.string", attrMessage),
		},
	}
}

type Kind int

const (
	KindAny Kind = iota
	KindLabel
	KindBool
	KindInt
	KindFloat
	KindString
	KindBytes
	KindList
	KindDict
	KindMessage
)

var kindToStr = map[Kind]string{
	KindAny:     "any",
	KindLabel:   "label",
	KindBool:    "bool",
	KindInt:     "int",
	KindFloat:   "float",
	KindString:  "string",
	KindBytes:   "bytes",
	KindList:    "list",
	KindDict:    "dict",
	KindMessage: "message",
}
var strToKind = func() map[string]Kind {
	m := make(map[string]Kind)
	for kind, str := range kindToStr {
		m[str] = kind
	}
	return m
}()

// KindType can be used as a key
type KindType struct {
	Kind    Kind
	KeyKind Kind   // dict
	ValKind Kind   // list or dict
	MsgType string // message protobuf type URL
}

func (t KindType) String() string {
	switch t.Kind {
	case KindList:
		return kindToStr[t.Kind] + "[" + kindToStr[t.ValKind] + "]"
	case KindDict:
		return kindToStr[t.Kind] + "<" + kindToStr[t.KeyKind] + "," + kindToStr[t.ValKind] + ">"
	case KindMessage:
		return kindToStr[t.Kind] + "<" + t.MsgType + ">"
	default:
		return kindToStr[t.Kind]
	}
}

func baseKind(value starlark.Value) Kind {
	switch (value).(type) {
	case *Label:
		return KindLabel
	case starlark.Bool:
		return KindBool
	case starlark.Int:
		return KindInt
	case starlark.Float:
		return KindFloat
	case starlark.String:
		return KindString
	case starlark.Bytes:
		return KindBytes
	case *starlark.List:
		return KindList
	case *starlark.Dict:
		return KindDict
	case *starlarkproto.Message:
		return KindMessage
	default:
		return KindAny
	}
}

func toKindType(value starlark.Value) KindType {
	kind := baseKind(value)
	switch kind {
	case KindList:
		x := value.(*starlark.List)
		n := x.Len()
		if n == 0 {
			return KindType{Kind: kind}
		}
		val := baseKind(x.Index(0))
		if val == KindList || val == KindDict {
			val = KindAny
		}
		return KindType{Kind: kind, ValKind: val}

	case KindDict:
		x := value.(*starlark.Dict)
		keys := x.Keys()
		if len(keys) == 0 {
			return KindType{Kind: kind}
		}
		key := keys[0]
		keyKind := baseKind(key)
		val, ok, err := x.Get(key)
		if err != nil || !ok {
			return KindType{Kind: kind, KeyKind: keyKind}
		}
		valKind := baseKind(val)
		return KindType{Kind: kind, KeyKind: keyKind, ValKind: valKind}

	case KindMessage:
		x := value.(*starlarkproto.Message)
		typ := string(x.ProtoReflect().Descriptor().FullName())
		return KindType{Kind: kind, MsgType: typ}
	default:
		return KindType{Kind: kind}
	}
}

// Attr defines a schema for a rules attribute.
type Attr struct {
	KindType
	Optional bool
	Doc      string
	Default  starlark.Value
	Values   starlark.Tuple // Oneof these values
}

type AttrValue struct {
	*Attr
	Value starlark.Value
}

type AttrName struct {
	*Attr
	Name string
}

type AttrNameValue struct {
	*Attr
	Name  string
	Value starlark.Value
}

func (a *Attr) String() string {
	var b strings.Builder
	b.WriteString("attr." + a.KindType.String())
	b.WriteString("(")
	b.WriteString("optional = ")
	b.WriteString(starlark.Bool(a.Optional).String())
	b.WriteString(", doc = ")
	b.WriteString(a.Doc)
	b.WriteString(", default = ")
	b.WriteString(a.Default.String())

	if len(a.Values) > 0 {
		b.WriteString(", values = ")
		b.WriteString(a.Values.String())
	}

	b.WriteString(")")
	return b.String()
}
func (a *Attr) Type() string { return "attr." + a.KindType.String() }

// func (a *Attr) AttrType() AttrType   { return a.Typ }
func (a *Attr) Freeze()               {} // immutable
func (a *Attr) Truth() starlark.Bool  { return true }
func (a *Attr) Hash() (uint32, error) { return 0, nil } // not hashable
//func (a *Attr) Hash() (uint32, error) {
//	var x, mult uint32 = 0x345678, 1000003
//	for _, elem := range []starlark.Value{
//		starlark.String(a.Typ),
//		starlark.String(a.Def.String()), // TODO
//		starlark.String(a.Doc),
//		starlark.Bool(a.Executable),
//		starlark.Bool(a.Mandatory),
//		starlark.Bool(a.AllowEmpty),
//		starlark.String(fmt.Sprintf("%v", a.AllowFiles)),
//		starlark.String(fmt.Sprintf("%v", a.Values)),
//	} {
//		y, err := elem.Hash()
//		if err != nil {
//			return 0, err
//		}
//		x = x ^ y*mult
//		mult += 82520
//	}
//	return x, nil
//}

/*func (a *Attr) IsValidType(value starlark.Value) bool {
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
}*/

/*func attrEqual(x, y *Attr, depth int) (bool, error) {
	return (x.Kind == y.Kind &&
		x.
	), nil
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
}*/

// Attribute attr.bool(default=False, doc=”", optional=False)
func attrBool(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		def      bool
		doc      string
		optional bool
	)
	if err := starlark.UnpackArgs(
		fnname, args, kwargs,
		"default?", &def, "doc?", &doc, "optional?", &optional,
	); err != nil {
		return nil, err
	}
	return &Attr{
		KindType: KindType{Kind: KindBool},
		Optional: optional,
		Doc:      doc,
		Default:  starlark.Bool(def),
		Values:   nil,
	}, nil
}

// Attribute attr.int(default=0, doc="", optoinal=False, values=[])
func attrInt(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		def      = starlark.MakeInt(0)
		doc      string
		optional bool
		values   starlark.Tuple
	)
	if err := starlark.UnpackArgs(
		fnname, args, kwargs,
		"default?", &def, "doc?", &doc, "optional?", &optional, "values?", &values,
	); err != nil {
		return nil, err
	}
	// TODO: validate values.
	return &Attr{
		KindType: KindType{Kind: KindInt},
		Optional: optional,
		Doc:      doc,
		Default:  def,
		Values:   values,
	}, nil
}

// Attribute attr.float(default=0, doc="", optional=False, values=[])
func attrFloat(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		def      = starlark.Float(0)
		doc      string
		optional bool
		values   starlark.Tuple
	)

	if err := starlark.UnpackArgs(
		fnname, args, kwargs,
		"default?", &def, "doc?", &doc, "optional?", &optional, "values?", &values,
	); err != nil {
		return nil, err
	}
	return &Attr{
		KindType: KindType{Kind: KindFloat},
		Optional: optional,
		Doc:      doc,
		Default:  def,
		Values:   values,
	}, nil
}

func attrString(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		def      starlark.String
		doc      string
		optional bool
		values   starlark.Tuple
	)
	if err := starlark.UnpackArgs(
		fnname, args, kwargs,
		"default?", &def, "doc?", &doc, "optional?", &optional, "values?", &values,
	); err != nil {
		return nil, err
	}
	return &Attr{
		KindType: KindType{Kind: KindString},
		Optional: optional,
		Doc:      doc,
		Default:  def,
		Values:   values,
	}, nil
}

// Attribute attr.float(default=0, doc="", optional=False, values=[])
func attrBytes(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		def      = starlark.Float(0)
		doc      string
		optional bool
		values   starlark.Tuple
	)

	if err := starlark.UnpackArgs(
		fnname, args, kwargs,
		"default?", &def, "doc?", &doc, "optional?", &optional, "values?", &values,
	); err != nil {
		return nil, err
	}
	return &Attr{
		KindType: KindType{Kind: KindBytes},
		Optional: optional,
		Doc:      doc,
		Default:  def,
		Values:   values,
	}, nil
}

// Attribute attr.list(val_kind="", optional=False, *, default=[], doc="")
func attrList(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		def      *starlark.List
		doc      string
		optional bool
		valStr   string
	)
	if err := starlark.UnpackArgs(
		fnname, args, kwargs,
		"val_kind", &valStr, "default?", &def, "doc?", &doc, "optional?", &optional,
	); err != nil {
		return nil, err
	}
	valKind := strToKind[valStr]

	switch valKind {
	case KindBool, KindInt, KindFloat, KindString, KindBytes, KindMessage:
		// scalar or message
	default:
		return nil, fmt.Errorf("invalid list value kind: %s", valStr)
	}

	return &Attr{
		KindType: KindType{Kind: KindList, ValKind: valKind},
		Optional: optional,
		Doc:      doc,
		Default:  def,
		Values:   nil,
	}, nil
}

// Attribute attr.dict(key_kind="", val_kind="", optional=False, default=[], doc="")
func attrDict(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		def      *starlark.Dict
		doc      string
		optional bool
		keyStr   string
		valStr   string
	)
	if err := starlark.UnpackArgs(
		fnname, args, kwargs,
		"key_kind", &keyStr, "val_kind", &valStr, "default?", &def, "doc?", &doc, "optional?", &optional,
	); err != nil {
		return nil, err
	}
	keyKind := strToKind[keyStr]
	valKind := strToKind[valStr]

	switch Kind(keyKind) {
	case KindBool, KindInt, KindFloat, KindString:
		// scalar or message
	default:
		return nil, fmt.Errorf("invalid dict key kind: %s", keyStr)
	}

	switch valKind {
	case KindBool, KindInt, KindFloat, KindString, KindBytes, KindMessage:
		// scalar or message
	default:
		return nil, fmt.Errorf("invalid list value kind: %s", valStr)
	}

	return &Attr{
		KindType: KindType{Kind: KindDict, KeyKind: keyKind, ValKind: valKind},
		Optional: optional,
		Doc:      doc,
		Default:  def,
		Values:   nil,
	}, nil
}

// Attribute attr.message(type="", optional=False, default=[], doc="")
func attrMessage(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		def      *starlark.Dict
		doc      string
		optional bool
		typStr   string
	)
	if err := starlark.UnpackArgs(
		fnname, args, kwargs,
		"type", &typStr, "default?", &def, "doc?", &doc, "optional?", &optional,
	); err != nil {
		return nil, err
	}
	// TODO: validate typStr?

	return &Attr{
		KindType: KindType{Kind: KindMessage, MsgType: typStr},
		Optional: optional,
		Doc:      doc,
		Default:  def,
		Values:   nil,
	}, nil
}

// Attrs -> AttrArgs
type Attrs struct {
	nameAttrs []AttrName
	frozen    bool
}

func (a *Attrs) String() string {
	buf := new(strings.Builder)
	buf.WriteString("attrs")
	buf.WriteByte('(')

	for i, v := range a.nameAttrs {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(v.Name)
		buf.WriteString(" = ")
		buf.WriteString(v.String())
	}
	buf.WriteByte(')')
	return buf.String()
}
func (a *Attrs) Truth() starlark.Bool  { return true } // even when empty
func (a *Attrs) Type() string          { return "attrs" }
func (a *Attrs) Hash() (uint32, error) { return 0, nil } // not hashable
func (a *Attrs) Freeze() {
	if a.frozen {
		return
	}
	a.frozen = true
	for _, v := range a.nameAttrs {
		v.Freeze()
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
	for _, v := range a.nameAttrs {
		if v.Name == name {
			return v.Attr, nil
		}
	}
	return nil, starlark.NoSuchAttrError(
		fmt.Sprintf("attrs has no .%s attribute", name))
}
func (a *Attrs) AttrNames() []string {
	names := make([]string, len(a.nameAttrs))
	for i, v := range a.nameAttrs {
		names[i] = v.Name
	}
	return names
}

//func attrsEqual(x, y *Attrs, depth int) (bool, error) {
//	if x.Len() != y.Len() {
//		return false, nil
//	}
//	for i, n := 0, x.Len(); i < n; i++ {
//		x, y := x.Index(i).(*Attr), y.Index(i).(*Attr)
//		eq, err := attrEqual(x, y, depth-1)
//		if !eq || err != nil {
//			return eq, err
//		}
//	}
//	return true, nil
//}

//	func (x *Attrs) CompareSameType(op syntax.Token, y_ starlark.Value, depth int) (bool, error) {
//		y := y_.(*Attrs)
//		switch op {
//		case syntax.EQL:
//			return attrsEqual(x, y, depth)
//		case syntax.NEQ:
//			eq, err := attrsEqual(x, y, depth)
//			return !eq, err
//		default:
//			return false, fmt.Errorf("%s %s %s not implemented", x.Type(), op, y.Type())
//		}
//	}
//func (a *Attrs) Index(i int) starlark.Value { return a.osd.Index(i) }
//func (a *Attrs) Len() int                   { return a.osd.Len() }

var nameAttr = AttrName{
	Attr: &Attr{
		KindType: KindType{Kind: KindString},
	},
	Name: "name",
}

func MakeAttrs(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs(fnname, args, nil); err != nil {
		return nil, err
	}

	nameAttrs := make([]AttrName, len(kwargs)+1)
	nameAttrs[0] = nameAttr

	for i, kwarg := range kwargs {
		name, _ := starlark.AsString(kwarg[0])
		if name == "name" || name == "" {
			return nil, fmt.Errorf("unexpected attribute name: %q", name)
		}
		a, ok := kwarg[1].(*Attr)
		if !ok {
			return nil, fmt.Errorf("unexpected attribute value type: %T", kwarg[1])
		}
		nameAttrs[i+1] = AttrName{
			Attr: a,
			Name: name,
		}
	}

	return &Attrs{
		nameAttrs: nameAttrs,
		frozen:    false,
	}, nil
}

func (a *Attrs) Get(name string) (*Attr, bool) {
	for _, v := range a.nameAttrs {
		if v.Name == name {
			return v.Attr, true
		}
	}
	return nil, false
}
