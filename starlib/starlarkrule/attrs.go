// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlarkrule

import (
	_ "embed"
	"fmt"
	"strings"

	"go.starlark.net/starlark"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"larking.io/api/actionpb"
	"larking.io/starlib/encoding/starlarkproto"
	"larking.io/starlib/starext"
	"larking.io/starlib/starlarkstruct"
)

func NewAttrModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "attr",
		Members: starlark.StringDict{
			"bool":   starext.MakeBuiltin("attr.bool", attrBool),
			"int":    starext.MakeBuiltin("attr.int", attrInt),
			"float":  starext.MakeBuiltin("attr.float", attrFloat),
			"string": starext.MakeBuiltin("attr.string", attrString),
			"bytes":  starext.MakeBuiltin("attr.bytes", attrBytes),
			"list":   starext.MakeBuiltin("attr.list", attrList),
			"dict":   starext.MakeBuiltin("attr.dict", attrDict),
			"proto":  starext.MakeBuiltin("attr.proto", attrProto),
		},
	}
}

var (
	TypeLabel  = (&actionpb.LabelValue{}).ProtoReflect().Descriptor().FullName()
	TypeBool   = (&wrapperspb.BoolValue{}).ProtoReflect().Descriptor().FullName()
	TypeInt    = (&wrapperspb.Int64Value{}).ProtoReflect().Descriptor().FullName()
	TypeFloat  = (&wrapperspb.FloatValue{}).ProtoReflect().Descriptor().FullName()
	TypeString = (&wrapperspb.StringValue{}).ProtoReflect().Descriptor().FullName()
	TypeBytes  = (&wrapperspb.BytesValue{}).ProtoReflect().Descriptor().FullName()
	TypeList   = (&actionpb.ListValue{}).ProtoReflect().Descriptor().FullName()
	TypeDict   = (&actionpb.DictValue{}).ProtoReflect().Descriptor().FullName()
)

// ToStar promotes known types back to starlark.
func ToStar(msg proto.Message) (starlark.Value, error) {
	switch x := (msg).(type) {
	case *actionpb.LabelValue:
		return ParseLabel(x.Value)
	case *wrapperspb.BoolValue:
		return starlark.Bool(x.Value), nil
	case *wrapperspb.Int64Value:
		return starlark.MakeInt64(x.Value), nil
	case *wrapperspb.FloatValue:
		return starlark.Float(x.Value), nil
	case *wrapperspb.StringValue:
		return starlark.String(x.Value), nil
	case *wrapperspb.BytesValue:
		return starlark.String(x.Value), nil
	case *actionpb.ListValue:
		elems := make([]starlark.Value, 0, len(x.Items))

		for _, item := range x.Items {
			y, err := item.UnmarshalNew()
			if err != nil {
				return nil, err
			}

			v, err := ToStar(y)
			if err != nil {
				return nil, err
			}
			elems = append(elems, v)
		}
		return starlark.NewList(elems), nil
	case *actionpb.DictValue:
		d := starlark.NewDict(len(x.Entries))

		for _, entry := range x.Entries {
			key, err := ToStar(entry.Key)
			if err != nil {
				return nil, err
			}
			val, err := ToStar(entry.Value)
			if err != nil {
				return nil, err
			}

			if err := d.SetKey(key, val); err != nil {
				return nil, err
			}
		}
		return d, nil
	default:
		return starlarkproto.NewMessage(msg.ProtoReflect(), nil, nil)
	}
}

func ToProto(value starlark.Value) (proto.Message, error) {
	switch x := (value).(type) {
	case *Label:
		return &actionpb.LabelValue{Value: x.String()}, nil
	case starlark.Bool:
		return &wrapperspb.BoolValue{Value: bool(x)}, nil
	case starlark.Int:
		var y int64
		if err := starlark.AsInt(value, &y); err != nil {
			return nil, err
		}
		return &wrapperspb.Int64Value{Value: y}, nil
	case starlark.Float:
		return &wrapperspb.DoubleValue{Value: float64(x)}, nil
	case starlark.String:
		return &wrapperspb.StringValue{Value: string(x)}, nil
	case starlark.Bytes:
		return &wrapperspb.BytesValue{Value: []byte(x)}, nil
	case *starlark.List:
		var items []*anypb.Any

		iter := x.Iterate()
		var p starlark.Value
		for iter.Next(&p) {
			y, err := ToProto(p)
			if err != nil {
				return nil, err
			}

			z, err := anypb.New(y)
			if err != nil {
				return nil, err
			}

			items = append(items, z)
		}

		return &actionpb.ListValue{Items: items}, nil
	case *starlark.Dict:
		var entries []*actionpb.EntryValue

		for _, item := range x.Items() {
			key, val := item[0], item[1]

			keyProto, err := ToProto(key)
			if err != nil {
				return nil, err
			}
			valProto, err := ToProto(val)
			if err != nil {
				return nil, err
			}

			keyAny, err := anypb.New(keyProto)
			if err != nil {
				return nil, err
			}
			valAny, err := anypb.New(valProto)
			if err != nil {
				return nil, err
			}

			entries = append(entries, &actionpb.EntryValue{
				Key:   keyAny,
				Value: valAny,
			})
		}

		return &actionpb.DictValue{Entries: entries}, nil
	case *starlarkproto.Message:
		return x.ProtoReflect().Interface(), nil
	default:
		return nil, fmt.Errorf("can't convert unknown type: %s", value.Type())
	}
}

func toType(value starlark.Value) (protoreflect.FullName, error) {
	switch x := (value).(type) {
	case *Label:
		return TypeLabel, nil
	case starlark.Bool:
		return TypeBool, nil
	case starlark.Int:
		return TypeInt, nil
	case starlark.Float:
		return TypeFloat, nil
	case starlark.String:
		return TypeString, nil
	case starlark.Bytes:
		return TypeBytes, nil
	case *starlark.List:
		return TypeList, nil
	case *starlark.Dict:
		return TypeDict, nil
	case *starlarkproto.Message:
		return x.ProtoReflect().Descriptor().FullName(), nil
	default:
		return "", fmt.Errorf("can't convert unknown type: %s", value.Type())
	}
}

// Attr defines a schema for a rules attribute.
type Attr struct {
	FullName protoreflect.FullName
	Optional bool
	Doc      string
	Default  starlark.Value
	Values   starlark.Tuple // Oneof these values
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
	b.WriteString("attr") // + a.KindType.String())
	b.WriteString("(")
	b.WriteString("type_url = ")
	b.WriteString(string(a.FullName))
	b.WriteString(", optional = ")
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
func (a *Attr) Type() string          { return "attr" } //+ a.KindType.String() }
func (a *Attr) Freeze()               {}                // immutable
func (a *Attr) Truth() starlark.Bool  { return true }
func (a *Attr) Hash() (uint32, error) { return 0, nil } // not hashable

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
		FullName: TypeBool,
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
		FullName: TypeInt,
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
		FullName: TypeFloat,
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
		FullName: TypeString,
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
		FullName: TypeBytes,
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
	)
	if err := starlark.UnpackArgs(
		fnname, args, kwargs,
		"default?", &def, "doc?", &doc, "optional?", &optional,
	); err != nil {
		return nil, err
	}

	return &Attr{
		FullName: TypeList,
		Optional: optional,
		Doc:      doc,
		Default:  def,
		Values:   nil,
	}, nil
}

// Attribute attr.dict(optional=False, default=[], doc="")
func attrDict(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		def      *starlark.Dict
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
		FullName: TypeDict,
		Optional: optional,
		Doc:      doc,
		Default:  def,
		Values:   nil,
	}, nil
}

// Attribute attr.proto(type="", optional=False, default=[], doc="")
func attrProto(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {

	var (
		def      *starlarkproto.Message
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
		FullName: protoreflect.FullName(typStr),
		Optional: optional,
		Doc:      doc,
		Default:  def,
		Values:   nil,
	}, nil
}

var nameAttr = AttrName{
	Attr: &Attr{
		FullName: TypeString,
		Doc:      "Name of rule",
	},
	Name: "name",
}
