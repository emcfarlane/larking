// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package starlarkproto provides support for protocol buffers.
package starlarkproto

import (
	"fmt"
	"sort"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/emcfarlane/larking/starext"
	"github.com/emcfarlane/larking/starlarkstruct"
)

const protokey = "protodescResolver"

func SetProtodescResolver(thread *starlark.Thread, resolver protodesc.Resolver) {
	thread.SetLocal(protokey, resolver)
}

func NewModule() *starlarkstruct.Module {
	p := NewProto()
	return &starlarkstruct.Module{
		Name: "proto",
		Members: starlark.StringDict{
			"file":           starext.MakeBuiltin("proto.file", p.File),
			"new":            starext.MakeBuiltin("proto.new", p.New),
			"marshal":        starext.MakeBuiltin("proto.marshal", p.Marshal),
			"unmarshal":      starext.MakeBuiltin("proto.unmarshal", p.Unmarshal),
			"marshal_json":   starext.MakeBuiltin("proto.marshal_json", p.MarshalJSON),
			"unmarshal_json": starext.MakeBuiltin("proto.unmarshal_json", p.UnmarshalJSON),
			"marshal_text":   starext.MakeBuiltin("proto.marshal_text", p.MarshalText),
			"unmarshal_text": starext.MakeBuiltin("proto.unmarshal_text", p.UnmarshalText),
		},
	}
}

type Proto struct {
	//resolver protodesc.Resolver
	types protoregistry.Types // TODO: wrap resolver to register extensions.
}

func (p *Proto) resolver(thread *starlark.Thread) protodesc.Resolver {
	if resolver, ok := thread.Local(protokey).(protodesc.Resolver); ok {
		return resolver
	}
	return protoregistry.GlobalFiles
}

func NewProto() *Proto {
	return &Proto{}
}

func (p *Proto) File(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 1, &name); err != nil {
		return nil, err
	}

	fileDesc, err := p.resolver(thread).FindFileByPath(name)
	if err != nil {
		return nil, err
	}
	return &Descriptor{desc: fileDesc}, nil
}

func (p *Proto) New(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 1, &name); err != nil {
		return nil, err
	}
	fullname := protoreflect.FullName(name)

	desc, err := p.resolver(thread).FindDescriptorByName(fullname)
	if err != nil {
		return nil, err
	}
	return &Descriptor{desc: desc}, nil
}

func (p *Proto) Marshal(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var msg *Message
	var options proto.MarshalOptions
	if err := starlark.UnpackPositionalArgs(
		fnname, args, kwargs, 1, &msg,
		"allow_partial?", &options.AllowPartial,
		"deterministic?", &options.Deterministic,
		"use_cache_size?", &options.UseCachedSize,
	); err != nil {
		return nil, err
	}
	data, err := options.Marshal(msg)
	if err != nil {
		return nil, err
	}
	return starlark.String(string(data)), nil
}

func (p *Proto) Unmarshal(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var str string
	var msg *Message
	options := proto.UnmarshalOptions{
		Resolver: &p.types, // TODO: types...
	}
	if err := starlark.UnpackPositionalArgs(
		fnname, args, kwargs, 2, &str, &msg,
		"merge?", &options.Merge,
		"allow_partial?", &options.AllowPartial,
		"discard_unknown?", &options.DiscardUnknown,
	); err != nil {
		return nil, err
	}
	if err := msg.checkMutable(fnname); err != nil {
		return nil, err
	}
	if err := proto.Unmarshal([]byte(str), msg); err != nil {
		return nil, err
	}
	return starlark.None, nil
}

func (p *Proto) MarshalJSON(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var msg *Message
	var options protojson.MarshalOptions
	if err := starlark.UnpackPositionalArgs(
		fnname, args, kwargs, 1, &msg,
		"multiline?", &options.Multiline,
		"indent?", &options.Indent,
		"allow_partial?", &options.AllowPartial,
		"use_proto_names?", &options.UseProtoNames,
		"use_enum_numbers?", &options.UseEnumNumbers,
		"emit_unpopulated?", &options.EmitUnpopulated,
	); err != nil {
		return nil, err
	}
	data, err := options.Marshal(msg)
	if err != nil {
		return nil, err
	}
	return starlark.String(string(data)), nil
}

func (p *Proto) UnmarshalJSON(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var str string
	var msg *Message
	options := protojson.UnmarshalOptions{
		Resolver: &p.types, // TODO: types...
	}
	if err := starlark.UnpackPositionalArgs(
		fnname, args, kwargs, 2, &str, &msg,
		"allow_partial?", &options.AllowPartial,
		"discard_unknown?", &options.DiscardUnknown,
	); err != nil {
		return nil, err
	}
	if err := msg.checkMutable(fnname); err != nil {
		return nil, err
	}
	if err := proto.Unmarshal([]byte(str), msg); err != nil {
		return nil, err
	}
	return starlark.None, nil
}

func (p *Proto) MarshalText(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var msg *Message
	var options prototext.MarshalOptions
	if err := starlark.UnpackPositionalArgs(
		fnname, args, kwargs, 1, &msg,
		"multiline?", &options.Multiline,
		"indent?", &options.Indent,
		"allow_partial?", &options.AllowPartial,
		"emit_unknown?", &options.EmitUnknown,
	); err != nil {
		return nil, err
	}
	data, err := options.Marshal(msg)
	if err != nil {
		return nil, err
	}
	return starlark.String(string(data)), nil
}

func (p *Proto) UnmarshalText(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var str string
	var msg *Message
	options := prototext.UnmarshalOptions{
		Resolver: &p.types, // TODO: types...
	}
	if err := starlark.UnpackPositionalArgs(
		fnname, args, kwargs, 2, &str, &msg,
		"allow_partial?", &options.AllowPartial,
		"discard_unknown?", &options.DiscardUnknown,
	); err != nil {
		return nil, err
	}
	if err := msg.checkMutable(fnname); err != nil {
		return nil, err
	}
	if err := proto.Unmarshal([]byte(str), msg); err != nil {
		return nil, err
	}
	return starlark.None, nil
}

func equalFullName(a, b protoreflect.FullName) error {
	if a != b {
		return fmt.Errorf("type mismatch %s != %s", a, b)
	}
	return nil
}

func isOwnType(v starlark.Value) bool {
	switch v.(type) {
	case *Descriptor, *Message, *List, *Map, Enum:
		return true
	default:
		return false
	}
}

type Descriptor struct {
	desc protoreflect.Descriptor

	frozen bool
	attrs  map[string]protoreflect.Descriptor
}

func NewDescriptor(desc protoreflect.Descriptor) *Descriptor { return &Descriptor{desc: desc} }

// Descriptor exports proto.Descriptor
func (d *Descriptor) Descriptor() protoreflect.Descriptor { return d.desc }

func (d *Descriptor) String() string        { return string(d.desc.Name()) }
func (d *Descriptor) Type() string          { return "proto.desc" }
func (d *Descriptor) Freeze()               { d.frozen = true }
func (d *Descriptor) Truth() starlark.Bool  { return d.desc != nil }
func (d *Descriptor) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: proto.desc") }
func (d *Descriptor) Name() string          { return string(d.desc.Name()) } // TODO
func (d *Descriptor) CallInternal(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	switch v := d.desc.(type) {
	case protoreflect.FileDescriptor:
		return nil, fmt.Errorf("proto: file descriptor not callable")

	case protoreflect.EnumDescriptor:
		if len(kwargs) > 0 {
			return nil, fmt.Errorf("unexpected kwargs")
		}
		if len(args) != 1 {
			return nil, fmt.Errorf("unexpected number of args")
		}
		vals := v.Values()
		return NewEnum(vals, args[0])

	case protoreflect.MessageDescriptor:
		msg := dynamicpb.NewMessage(v)
		return NewMessage(msg, args, kwargs)

	default:
		return nil, fmt.Errorf("proto: desc missing call type %T", v)
	}
}

func (d *Descriptor) getAttrs() map[string]protoreflect.Descriptor {
	if d.attrs != nil {
		return d.attrs
	}
	m := make(map[string]protoreflect.Descriptor)

	switch v := d.desc.(type) {
	case protoreflect.FileDescriptor:
		for i, eds := 0, v.Enums(); i < eds.Len(); i++ {
			ed := eds.Get(i)
			m[string(ed.Name())] = ed
		}
		for i, mds := 0, v.Messages(); i < mds.Len(); i++ {
			md := mds.Get(i)
			m[string(md.Name())] = md
		}
		for i, eds := 0, v.Extensions(); i < eds.Len(); i++ {
			ed := eds.Get(i)
			m[string(ed.Name())] = ed
		}
		for i, sds := 0, v.Services(); i < sds.Len(); i++ {
			sd := sds.Get(i)
			m[string(sd.Name())] = sd
		}

	case protoreflect.EnumDescriptor:
		for i, eds := 0, v.Values(); i < eds.Len(); i++ {
			evd := eds.Get(i)
			m[string(evd.Name())] = evd
		}

	case protoreflect.MessageDescriptor:
		for i, eds := 0, v.Enums(); i < eds.Len(); i++ {
			ed := eds.Get(i)
			m[string(ed.Name())] = ed
		}
		for i, mds := 0, v.Messages(); i < mds.Len(); i++ {
			md := mds.Get(i)
			m[string(md.Name())] = md
		}
		for i, ods := 0, v.Oneofs(); i < ods.Len(); i++ {
			od := ods.Get(i)
			m[string(od.Name())] = od
		}

	case protoreflect.ServiceDescriptor:
		for i, mds := 0, v.Methods(); i < mds.Len(); i++ {
			md := mds.Get(i)
			m[string(md.Name())] = md
		}

	default:
		panic(fmt.Sprintf("proto: desc missing attr type %T", v))
	}

	if !d.frozen {
		d.attrs = m
	}
	return m
}

func (d *Descriptor) Attr(name string) (starlark.Value, error) {
	// TODO: can this just use the resolver?
	attrs := d.getAttrs()
	desc, ok := attrs[name]
	if !ok {
		return nil, nil
	}
	// Special descriptor type handling
	switch v := desc.(type) {
	case protoreflect.EnumValueDescriptor:
		return Enum{edesc: v}, nil
	default:
		return &Descriptor{desc: desc}, nil
	}
}

func (d *Descriptor) AttrNames() []string {
	var names []string
	for name := range d.getAttrs() {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Message represents a proto.Message as a starlark.Value.
type Message struct {
	msg protoreflect.Message

	frozen bool
	refs   map[protoreflect.Name]starlark.Value // mutable live references
}

// ProtoReflect implements proto.Message
func (m *Message) ProtoReflect() protoreflect.Message { return m.msg }

// Type conversions rules:
//
//  ═══════════════╤════════════════════════════════════
//  Starlark type  │ Protobuf Type
//  ═══════════════╪════════════════════════════════════
//  NoneType       │ MessageKind, GroupKind
//  Bool           │ BoolKind
//  Int            │ Int32Kind, Sint32Kind, Sfixed32Kind,
//                 │ Int64Kind, Sint64Kind, Sfixed64Kind,
//                 │ Uint32Kind, Fixed32Kind,
//                 │ Uint64Kind, Fixed64Kind
//  Float          │ FloatKind, DoubleKind
//  String         │ StringKind, BytesKind
//  *List          │ List<Kind>
//  Tuple          │ n/a
//  *Dict          │ Map<Kind><Kind>
//  *Set           │ n/a
//
func protoToStar(v protoreflect.Value, fd protoreflect.FieldDescriptor) starlark.Value {
	switch v := v.Interface().(type) {
	case nil:
		return starlark.None
	case bool:
		return starlark.Bool(v)
	case int32:
		return starlark.MakeInt(int(v))
	case int64:
		return starlark.MakeInt(int(v))
	case uint32:
		return starlark.MakeInt(int(v))
	case uint64:
		return starlark.MakeInt(int(v))
	case float32:
		return starlark.Float(float64(v))
	case float64:
		return starlark.Float(v)
	case string:
		return starlark.String(v)
	case []byte:
		return starlark.String(v)
	case protoreflect.EnumNumber:
		evdesc := fd.Enum().Values().ByNumber(v)
		if evdesc == nil {
			evdesc = fd.DefaultEnumValue() // TODO: error?
		}
		return Enum{edesc: evdesc}
	case protoreflect.List:
		return &List{list: v, fd: fd}
	case protoreflect.Message:
		return &Message{msg: v}
	case protoreflect.Map:
		return &Map{m: v, keyfd: fd.MapKey(), valfd: fd.MapValue()}
	default:
		panic(fmt.Sprintf("unhandled proto type %s %T", v, v))
	}
}

func starToProtoMessage(v starlark.Value, val *protoreflect.Value) error {
	switch v := v.(type) {
	case *Message:
		msg := val.Message()
		if err := equalFullName(msg.Descriptor().FullName(), v.msg.Descriptor().FullName()); err != nil {
			return err
		}
		*val = protoreflect.ValueOfMessage(v.msg)
		return nil
	case starlark.NoneType:
		msg := val.Message()
		*val = protoreflect.ValueOfMessage(msg.Type().Zero()) // RO
		return nil
	case starlark.IterableMapping:
		msg := val.Message()
		m := Message{msg: msg} // wrap for set

		for _, kv := range v.Items() {
			key, ok := kv[0].(starlark.String)
			if !ok {
				return fmt.Errorf("proto: invalid key type %s", kv[0].Type())
			}
			if err := m.SetField(string(key), kv[1]); err != nil {
				return err
			}
		}
		return nil
	case starlark.HasAttrs:
		msg := val.Message()
		m := Message{msg: msg} // wrap for set

		for _, name := range v.AttrNames() {
			val, err := v.Attr(name)
			if err != nil {
				return err
			}
			if err := m.SetField(name, val); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("proto: unknown type conversion %s<%T> to proto.message", v, v)
	}
}

func starToProto(v starlark.Value, fd protoreflect.FieldDescriptor, val *protoreflect.Value) error {
	switch kind := fd.Kind(); kind {
	case protoreflect.BoolKind:
		if b, ok := v.(starlark.Bool); ok {
			*val = protoreflect.ValueOfBool(bool(b))
			return nil
		}
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		if x, err := starlark.NumberToInt(v); err == nil {
			v, err := starlark.AsInt32(x)
			if err != nil {
				return err
			}
			*val = protoreflect.ValueOfInt32(int32(v))
			return nil
		}
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		if x, err := starlark.NumberToInt(v); err == nil {
			v, _ := x.Int64()
			*val = protoreflect.ValueOfInt64(int64(v))
			return nil
		}
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		if x, err := starlark.NumberToInt(v); err == nil {
			v, _ := x.Uint64()
			*val = protoreflect.ValueOfUint32(uint32(v))
			return nil
		}
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		if x, err := starlark.NumberToInt(v); err == nil {
			v, _ := x.Uint64()
			*val = protoreflect.ValueOfUint64(uint64(v))
			return nil
		}
	case protoreflect.FloatKind:
		if x, ok := starlark.AsFloat(v); ok {
			*val = protoreflect.ValueOfFloat32(float32(x))
			return nil
		}
	case protoreflect.DoubleKind:
		if x, ok := starlark.AsFloat(v); ok {
			*val = protoreflect.ValueOfFloat64(float64(x))
			return nil
		}
	case protoreflect.StringKind:
		if x, ok := v.(starlark.String); ok {
			*val = protoreflect.ValueOfString(string(x))
			return nil
		}
	case protoreflect.BytesKind:
		if x, ok := v.(starlark.String); ok {
			*val = protoreflect.ValueOfBytes([]byte(x))
			return nil
		}
	case protoreflect.EnumKind:
		switch v := v.(type) {
		case starlark.String:
			enumVal := fd.Enum().Values().ByName(protoreflect.Name(string(v)))
			if enumVal == nil {
				return fmt.Errorf("proto: enum has no %s value", v)
			}
			*val = protoreflect.ValueOfEnum(enumVal.Number())
			return nil
		case starlark.Int, starlark.Float:
			i, err := starlark.NumberToInt(v)
			if err != nil {
				return err
			}
			x, ok := i.Int64()
			if !ok {
				return fmt.Errorf("proto: enum has no %s value", v)
			}
			*val = protoreflect.ValueOfEnum(protoreflect.EnumNumber(int32(x)))
			return nil
		case Enum:
			if err := equalFullName(v.edesc.Parent().FullName(), fd.Enum().FullName()); err != nil {
				return err
			}
			*val = protoreflect.ValueOfEnum(v.edesc.Number())
			return nil
		}
	case protoreflect.MessageKind:
		if fd.IsMap() {
			switch v := v.(type) {
			case *Map:
				// TODO: maps just need the same type?
				if err := equalFullName(v.keyfd.FullName(), fd.MapKey().FullName()); err != nil {
					return err
				}
				if err := equalFullName(v.valfd.FullName(), fd.MapValue().FullName()); err != nil {
					return err
				}
				*val = protoreflect.ValueOfMap(v.m)
				return nil
			case starlark.IterableMapping:
				mm := val.Map()
				kfd := fd.MapKey()
				vfd := fd.MapValue()

				items := v.Items()
				for _, item := range items {
					mval := mm.NewValue()
					if err := starToProto(item[0], kfd, &mval); err != nil {
						return err
					}
					mkey := mval.MapKey()

					vval := mm.Mutable(mkey)
					if err := starToProto(item[1], vfd, &vval); err != nil {
						return err
					}

					mm.Set(mkey, vval)
				}
				return nil
			}
			break
		}
		return starToProtoMessage(v, val)
	default:
		panic(fmt.Sprintf("unknown kind %q", kind))
	}
	return fmt.Errorf("proto: unknown type conversion %s<%T> to %s", v, v, fd.Kind().String())
}

func starToProtos(v starlark.Value, fd protoreflect.FieldDescriptor, val *protoreflect.Value) error {
	if !fd.IsList() {
		return starToProto(v, fd, val)
	}

	switch v := v.(type) {
	case *List:
		// TODO: check field types assertion makes sense.
		if err := equalFullName(v.fd.FullName(), fd.FullName()); err != nil {
			return err
		}
		*val = protoreflect.ValueOfList(v.list)
		// Starlark type is wrapped in ref by caller.
		return nil

	case starlark.Indexable:
		l := val.List()
		for i := 0; i < v.Len(); i++ {
			elem := l.NewElement()
			if err := starToProto(v.Index(i), fd, &elem); err != nil {
				return err
			}
			l.Append(elem)
		}
		return nil
	case starlark.Iterable:
		l := val.List()
		iter := v.Iterate()
		defer iter.Done()

		var p starlark.Value
		for iter.Next(&p) {
			elem := l.NewElement()
			if err := starToProto(p, fd, &elem); err != nil {
				return err
			}
			l.Append(elem)
		}
		return nil
	}
	return fmt.Errorf("proto: unknown repeated type conversion %s", v.Type())
}

func (m *Message) toValue(fd protoreflect.FieldDescriptor) starlark.Value {
	return protoToStar(m.msg.Get(fd), fd)
}

func (m *Message) isMutableType(fd protoreflect.FieldDescriptor) bool {
	if fd.IsMap() || fd.IsList() {
		if m.refs == nil {
			m.refs = make(map[protoreflect.Name]starlark.Value)
		}
		if _, ok := m.refs[fd.Name()]; !ok {
			m.refs[fd.Name()] = protoToStar(m.msg.Mutable(fd), fd)
		}
		return true
	}
	return fd.Kind() == protoreflect.MessageKind && m.msg.Has(fd)
}

func (m *Message) mutable(fd protoreflect.FieldDescriptor) starlark.Value {
	if m.isMutableType(fd) {
		return m.refs[fd.Name()] // SetField creates reference
	}
	return m.toValue(fd)
}

func (m *Message) checkMutable(verb string) error {
	if m.frozen {
		return fmt.Errorf("cannot %s frozen message", verb)
	}
	if !m.msg.IsValid() {
		return fmt.Errorf("cannot %s non mutable message", verb)
	}
	return nil
}

func NewMessage(msg protoreflect.Message, args starlark.Tuple, kwargs []starlark.Tuple) (*Message, error) {
	hasArgs := len(args) > 0
	hasKwargs := len(kwargs) > 0

	if hasArgs && len(args) > 1 {
		return nil, fmt.Errorf("unexpected number of args")
	}

	if hasArgs && hasKwargs {
		return nil, fmt.Errorf("unxpected args and kwargs")
	}

	// TODO: clear?
	m := &Message{
		msg: msg,
	}
	if hasArgs {
		val := protoreflect.ValueOfMessage(msg)
		if err := starToProtoMessage(args[0], &val); err != nil {
			return nil, err
		}
		return m, nil
	}

	for _, kwarg := range kwargs {
		k := string(kwarg[0].(starlark.String))
		v := kwarg[1]

		if err := m.SetField(k, v); err != nil {
			return nil, err
		}
	}
	return m, nil
}

func (m *Message) String() string {
	desc := m.msg.Descriptor()
	buf := new(strings.Builder)
	buf.WriteString(string(desc.Name()))

	buf.WriteByte('(')
	if m.msg.IsValid() {
		fds := desc.Fields()
		for i := 0; i < fds.Len(); i++ {
			if i > 0 {
				buf.WriteString(", ")
			}
			fd := fds.Get(i)
			buf.WriteString(string(fd.Name()))
			buf.WriteString(" = ")
			v := m.toValue(fd)
			buf.WriteString(v.String())
		}
	} else {
		buf.WriteString("None")
	}
	buf.WriteByte(')')
	return buf.String()
}

func (m *Message) Type() string         { return "proto.message" }
func (m *Message) Truth() starlark.Bool { return starlark.Bool(m.msg.IsValid()) }
func (m *Message) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: proto.message")
}
func (m *Message) Freeze() {
	if !m.frozen {
		m.frozen = true
		for _, v := range m.refs {
			v.Freeze()
		}
	}
}

// Attr returns the value of the specified field.
func (m *Message) Attr(name string) (starlark.Value, error) {
	fd, err := m.fieldDesc(name)
	if err != nil {
		return nil, err
	}
	return m.mutable(fd), nil // attr can mutate
}

func (x *Message) Binary(op syntax.Token, y starlark.Value, side starlark.Side) (starlark.Value, error) {
	return nil, nil // unhandled
}

// AttrNames returns a new sorted list of the message fields.
func (m *Message) AttrNames() []string {
	desc := m.msg.Descriptor()
	fds := desc.Fields()
	ods := desc.Oneofs()
	names := make([]string, fds.Len()+ods.Len())
	for i := 0; i < fds.Len(); i++ {
		fd := fds.Get(i)
		names[i] = string(fd.Name())
	}
	offset := fds.Len()
	for i := 0; i < ods.Len(); i++ {
		od := ods.Get(i)
		names[offset+i] = string(od.Name())
	}
	sort.Strings(names) // TODO: sort by protobuf number
	return names
}

func (m *Message) fieldDesc(name string) (protoreflect.FieldDescriptor, error) {
	desc := m.msg.Descriptor()
	if fd := desc.Fields().ByName(protoreflect.Name(name)); fd != nil {
		return fd, nil
	}

	if od := desc.Oneofs().ByName(protoreflect.Name(name)); od != nil {
		return m.msg.WhichOneof(od), nil
	}

	return nil, starlark.NoSuchAttrError(
		fmt.Sprintf("%s has no .%s attribute", desc.Name(), name),
	)
}

func (m *Message) SetField(name string, val starlark.Value) error {
	if err := m.checkMutable("set field"); err != nil {
		return err
	}
	fd, err := m.fieldDesc(name)
	if err != nil {
		return err
	}

	if val == starlark.None {
		m.msg.Clear(fd)
		return nil
	}

	v := m.msg.NewField(fd)
	if err := starToProtos(val, fd, &v); err != nil {
		return err
	}

	m.msg.Set(fd, v)
	if m.isMutableType(fd) {
		if m.refs == nil {
			m.refs = make(map[protoreflect.Name]starlark.Value)
		}
		if !isOwnType(val) {
			val = protoToStar(v, fd)
		}
		m.refs[fd.Name()] = val
	}
	return nil
}

func (x *Message) CompareSameType(op syntax.Token, y_ starlark.Value, depth int) (bool, error) {
	y := y_.(*Message)
	switch op {
	case syntax.EQL:
		return proto.Equal(x, y), nil
	case syntax.NEQ:
		return !proto.Equal(x, y), nil
	case syntax.LE, syntax.LT, syntax.GE, syntax.GT:
		return false, fmt.Errorf("%v not implemented", op)
	default:
		panic(op)
	}
}

// List represents a repeated field as a starlark.List.
type List struct {
	list protoreflect.List
	fd   protoreflect.FieldDescriptor

	frozen    bool
	itercount uint32
	refs      []starlark.Value // mirrors list on mutable types
}

type listAttr func(l *List) starlark.Value

// methods from starlark/library.go
var listAttrs = map[string]listAttr{
	"append": func(l *List) starlark.Value { return starext.MakeMethod(l, "append", l.append) },
	"clear":  func(l *List) starlark.Value { return starext.MakeMethod(l, "clear", l.clear) },
	"extend": func(l *List) starlark.Value { return starext.MakeMethod(l, "extend", l.extend) },
	"index":  func(l *List) starlark.Value { return starext.MakeMethod(l, "index", l.index) },
	"insert": func(l *List) starlark.Value { return starext.MakeMethod(l, "insert", l.insert) },
	"pop":    func(l *List) starlark.Value { return starext.MakeMethod(l, "pop", l.pop) },
	"remove": func(l *List) starlark.Value { return starext.MakeMethod(l, "remove", l.remove) },
}

func (l *List) Attr(name string) (starlark.Value, error) {
	if a := listAttrs[name]; a != nil {
		return a(l), nil
	}
	return nil, nil

}
func (l *List) AttrNames() []string {
	names := make([]string, 0, len(listAttrs))
	for name := range listAttrs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (l *List) String() string {
	buf := new(strings.Builder)
	buf.WriteByte('[')
	if l.list.IsValid() {
		for i := 0; i < l.Len(); i++ {
			if i > 0 {
				buf.WriteString(", ")
			}
			buf.WriteString(l.Index(i).String())
		}
	}
	buf.WriteByte(']')
	return buf.String()
}

func (l *List) isMutableType() bool {
	return l.fd.Kind() == protoreflect.MessageKind
}

func (l *List) Freeze() {
	if !l.frozen {
		l.frozen = true
		for _, v := range l.refs {
			v.Freeze()
		}
	}
}

func (l *List) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: proto.list")
}

func (l *List) checkMutable(verb string) error {
	if l.frozen {
		return fmt.Errorf("cannot %s frozen list", verb)
	}
	if l.itercount > 0 {
		return fmt.Errorf("cannot %s list during iteration", verb)
	}
	if !l.list.IsValid() {
		return fmt.Errorf("cannot %s non mutable list", verb)
	}
	return nil
}

func (l *List) Index(i int) starlark.Value {
	if i < len(l.refs) {
		return l.refs[i] // mutable refs
	}
	return protoToStar(l.list.Get(i), l.fd)
}

type listIterator struct {
	l    *List
	vals []starlark.Value
	i    int
}

func (it *listIterator) Next(p *starlark.Value) bool {
	if it.i < len(it.vals) {
		*p = it.vals[it.i]
		it.i++
		return true
	}
	return false
}

func (it *listIterator) Done() {
	if !it.l.frozen {
		it.l.itercount--
	}
}

func (l *List) Iterate() starlark.Iterator {
	if !l.frozen {
		l.itercount++
	}
	if len(l.refs) > 0 {
		return &listIterator{l: l, vals: l.refs}
	}
	vals := make([]starlark.Value, l.list.Len())
	for i := 0; i < l.list.Len(); i++ {
		val := l.list.Get(i)
		vals[i] = protoToStar(val, l.fd)
	}
	return &listIterator{l: l, vals: vals}
}

// From Hacker's Delight, section 2.8.
func signum(x int64) int { return int(uint64(x>>63) | uint64(-x)>>63) }

// Slice copies values to a starlark.List
func (l *List) Slice(start, end, step int) starlark.Value {
	sign := signum(int64(step))

	var elems []starlark.Value
	for i := start; signum(int64(end-i)) == sign; i += step {
		elems = append(elems, l.Index(i))
	}
	return starlark.NewList(elems)
}

func (l *List) Clear() error {
	if err := l.checkMutable("clear"); err != nil {
		return err
	}
	if l.list.Len() > 0 {
		l.list.Truncate(0)
		l.refs = nil
	}
	return nil
}

func (l *List) Type() string         { return l.fd.Kind().String() }
func (l *List) Len() int             { return l.list.Len() }
func (l *List) Truth() starlark.Bool { return l.Len() > 0 }

func (l *List) SetIndex(i int, v starlark.Value) error {
	if err := l.checkMutable("assign to element of"); err != nil {
		return err
	}

	val := l.list.NewElement()
	if err := starToProto(v, l.fd, &val); err != nil {
		return err
	}

	l.list.Set(i, val)
	if l.isMutableType() {
		if !isOwnType(v) {
			v = protoToStar(val, l.fd)
		}
		l.refs[i] = v
	}
	return nil
}

func (l *List) Append(v starlark.Value) error {
	if err := l.checkMutable("append to"); err != nil {
		return err
	}
	val := l.list.NewElement()
	if err := starToProto(v, l.fd, &val); err != nil {
		return err
	}

	l.list.Append(val)
	if l.isMutableType() {
		l.refs = append(l.refs, protoToStar(val, l.fd))
	}
	return nil
}

func (l *List) Pop(i int) (starlark.Value, error) {
	v := l.Index(i)
	n := l.Len()

	// shift list after index
	for j := i; j < n-1; j++ {
		val := l.list.Get(j + 1)
		l.list.Set(j, val)
	}
	l.list.Truncate(n - 1)

	if l.isMutableType() {
		l.refs = append(l.refs[:i], l.refs[i+1:]...)
	}
	return v, nil

}

func (v *List) append(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var object starlark.Value
	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 1, &object); err != nil {
		return nil, err
	}
	if err := v.Append(object); err != nil {
		return nil, err
	}
	return starlark.None, nil
}

func (v *List) clear(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 0); err != nil {
		return nil, err
	}
	if err := v.Clear(); err != nil {
		return nil, err
	}
	return starlark.None, nil
}

func (v *List) extend(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var iterable starlark.Iterable
	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 1, &iterable); err != nil {
		return nil, err
	}
	iter := iterable.Iterate()
	var p starlark.Value
	for iter.Next(&p) {
		if err := v.Append(p); err != nil {
			return nil, err
		}
	}
	return starlark.None, nil
}

func outOfRange(i, n int, x starlark.Value) error {
	if n == 0 {
		return fmt.Errorf("index %d out of range: empty %s", i, x.Type())
	} else {
		return fmt.Errorf("%s index %d out of range [%d:%d]", x.Type(), i, -n, n-1)
	}
}

func absIndex(i, len int) int {
	if i < 0 {
		i += len // negative offset
	}
	// clamp [0:len]
	if i < 0 {
		i = 0
	} else if i > len {
		i = len
	}
	return i
}

func asIndex(v starlark.Value, len int, result *int) (err error) {
	if v != nil && v != starlark.None {
		*result, err = starlark.AsInt32(v)
		if err != nil {
			return err
		}
		*result = absIndex(*result, len)
	}
	return nil
}

func (v *List) index(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var value, start_, end_ starlark.Value
	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 1, &value, &start_, &end_); err != nil {
		return nil, err
	}

	len := v.Len()
	start := 0
	if err := asIndex(start_, len, &start); err != nil {
		return nil, err
	}

	end := len
	if err := asIndex(end_, len, &end); err != nil {
		return nil, err
	}

	// find
	for i := start; i < end; i++ {
		if ok, err := starlark.Equal(v.Index(i), value); ok {
			return starlark.MakeInt(i), nil
		} else if err != nil {
			return nil, fmt.Errorf("%s: %w", fnname, err)
		}
	}
	return nil, fmt.Errorf("%s: value not in list", fnname)
}

func (v *List) insert(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var index int
	var object starlark.Value
	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 2, &index, &object); err != nil {
		return nil, err
	}
	if err := v.checkMutable("insert into"); err != nil {
		return nil, fmt.Errorf("%s: %w", v.Type(), err)
	}

	len := v.Len()
	index = absIndex(index, len)
	if index >= len {
		if err := v.Append(object); err != nil {
			return nil, err
		}
		return starlark.None, nil
	}

	val := v.list.NewElement()
	if err := starToProto(object, v.fd, &val); err != nil {
		return nil, err
	}

	x := object
	if v.isMutableType() {
		if !isOwnType(x) {
			x = protoToStar(val, v.fd)
		}
	}

	for i := index; i < len; i++ {
		swap := v.list.Get(i)
		v.list.Set(i, val)
		val = swap

		if v.isMutableType() {
			swapRef := v.refs[i]
			v.refs[i] = v
			x = swapRef
		}
	}

	v.list.Append(val)
	if v.isMutableType() {
		v.refs = append(v.refs, x)
	}

	return starlark.None, nil
}

func (v *List) pop(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	n := v.Len()
	i := n - 1
	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 0, &i); err != nil {
		return nil, err
	}
	if err := v.checkMutable("pop from"); err != nil {
		return nil, fmt.Errorf("%s: %w", fnname, err)
	}
	origI := i
	if i < 0 {
		i += n
	}
	if i < 0 || i >= n {
		return nil, fmt.Errorf("%s: %w", fnname, outOfRange(origI, n, v))
	}
	return v.Pop(i)
}

func (v *List) remove(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var value starlark.Value
	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 1, &value); err != nil {
		return nil, err
	}
	if err := v.checkMutable("remove from"); err != nil {
		return nil, fmt.Errorf("%s: %w", v.Type(), err)
	}

	// find
	for i := 0; i < v.Len(); i++ {
		if ok, err := starlark.Equal(v.Index(i), value); ok {
			// pop
			if _, err := v.Pop(i); err != nil {
				return nil, err
			}
			return starlark.None, nil

		} else if err != nil {
			return nil, fmt.Errorf("%s: %w", fnname, err)
		}
	}
	return nil, fmt.Errorf("%s: element not found", fnname)
}

// Enum is the type of a protobuf enum.
type Enum struct {
	edesc protoreflect.EnumValueDescriptor
}

func NewEnum(enum protoreflect.EnumValueDescriptors, arg starlark.Value) (Enum, error) {
	switch v := arg.(type) {
	case starlark.String:
		edesc := enum.ByName(protoreflect.Name(v))
		if edesc == nil {
			return Enum{}, fmt.Errorf("proto: enum not found")
		}
		return Enum{edesc: edesc}, nil

	case starlark.Int:
		n, _ := v.Int64() // TODO: checks?
		edesc := enum.ByNumber(protoreflect.EnumNumber(n))
		return Enum{edesc: edesc}, nil

	case Enum:
		return Enum{edesc: v.edesc}, nil

	default:
		return Enum{}, fmt.Errorf("unsupported type %s", arg.Type())
	}
}

func (e Enum) String() string        { return string(e.edesc.Name()) }
func (e Enum) Type() string          { return "proto.enum" }
func (e Enum) Freeze()               {} // immutable
func (e Enum) Truth() starlark.Bool  { return e.edesc.Number() > 0 }
func (e Enum) Hash() (uint32, error) { return uint32(e.edesc.Number()), nil }
func (x Enum) CompareSameType(op syntax.Token, y_ starlark.Value, depth int) (bool, error) {
	y := y_.(Enum)
	if err := equalFullName(x.edesc.Parent().FullName(), y.edesc.Parent().FullName()); err != nil {
		return false, err
	}
	i, j := x.edesc.Number(), y.edesc.Number()
	switch op {
	case syntax.EQL:
		return i == j, nil
	case syntax.NEQ:
		return i != j, nil
	case syntax.LE:
		return i <= j, nil
	case syntax.LT:
		return i < j, nil
	case syntax.GE:
		return i >= j, nil
	case syntax.GT:
		return i > j, nil
	default:
		panic(op)
	}
}

type Map struct {
	m     protoreflect.Map
	keyfd protoreflect.FieldDescriptor
	valfd protoreflect.FieldDescriptor

	frozen    bool
	itercount uint32
	refs      map[interface{}]starlark.Value // interface{} == protobuf.MapKey.Interface()
}

func (m *Map) Clear() error {
	m.m.Range(func(key protoreflect.MapKey, val protoreflect.Value) bool {
		m.m.Clear(key)
		return true
	})
	m.refs = nil // TODO: iterate?
	return nil
}
func (m *Map) parseKey(k starlark.Value) (protoreflect.MapKey, error) {
	var keyval protoreflect.Value
	if err := starToProto(k, m.keyfd, &keyval); err != nil {
		return protoreflect.MapKey{}, err
	}
	return keyval.MapKey(), nil
}
func (m *Map) toValue(key protoreflect.MapKey) (starlark.Value, bool) {
	if m.isMutableType() && m.m.Has(key) {
		return m.refs[key.Interface()], true
	}
	val := m.m.Get(key)
	if !val.IsValid() {
		return starlark.None, false
	}
	return protoToStar(val, m.valfd), true
}
func (m *Map) Delete(k starlark.Value) (v starlark.Value, found bool, err error) {
	key, err := m.parseKey(k)
	if err != nil {
		return nil, false, err
	}

	v, found = m.toValue(key)
	if found {
		m.m.Clear(key)
	}
	return v, found, nil
}
func (m *Map) Get(k starlark.Value) (v starlark.Value, found bool, err error) {
	key, err := m.parseKey(k)
	if err != nil {
		return nil, false, err
	}

	v, found = m.toValue(key)
	return v, found, nil
}

type byTuple []starlark.Tuple

func (a byTuple) Len() int      { return len(a) }
func (a byTuple) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a byTuple) Less(i, j int) bool {
	c := a[i][0].(starlark.Comparable)
	ok, err := c.CompareSameType(syntax.LT, a[j][0], 1)
	if err != nil {
		panic(err)
	}
	return ok
}

func (m *Map) Items() []starlark.Tuple {
	v := make([]starlark.Tuple, 0, m.Len())
	if m.isMutableType() {
		for key, val := range m.refs {
			v = append(v, starlark.Tuple{
				protoToStar(protoreflect.ValueOf(key), m.keyfd),
				val,
			})
		}
	} else {
		m.m.Range(func(key protoreflect.MapKey, val protoreflect.Value) bool {
			v = append(v, starlark.Tuple{
				protoToStar(key.Value(), m.keyfd),
				protoToStar(val, m.valfd),
			})
			return true
		})
	}
	sort.Sort(byTuple(v))
	return v
}

type byValue []starlark.Value

func (a byValue) Len() int      { return len(a) }
func (a byValue) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a byValue) Less(i, j int) bool {
	c := a[i].(starlark.Comparable)
	ok, err := c.CompareSameType(syntax.LT, a[j], 1)
	if err != nil {
		panic(err)
	}
	return ok
}

func (m *Map) Keys() []starlark.Value {
	v := make([]starlark.Value, 0, m.Len())
	if m.isMutableType() {
		for key := range m.refs {
			v = append(
				v, protoToStar(protoreflect.ValueOf(key), m.keyfd),
			)
		}
	} else {
		m.m.Range(func(key protoreflect.MapKey, _ protoreflect.Value) bool {
			v = append(v, protoToStar(key.Value(), m.keyfd))
			return true
		})
	}
	sort.Sort(byValue(v))
	return v
}
func (m *Map) Len() int {
	return m.m.Len()
}

type keyIterator struct {
	m    *Map
	keys []starlark.Value // copy
	i    int
}

func (ki *keyIterator) Next(k *starlark.Value) bool {
	if ki.i < len(ki.keys) {
		*k = ki.keys[ki.i]
		ki.i++
		return true
	}
	return false
}

func (ki *keyIterator) Done() {
	if !ki.m.frozen {
		ki.m.itercount--
	}
}

func (m *Map) Iterate() starlark.Iterator {
	if !m.frozen {
		m.itercount--
	}
	return &keyIterator{m: m, keys: m.Keys()}
}
func (m *Map) SetKey(k, v starlark.Value) error {
	if err := m.checkMutable("set"); err != nil {
		return err
	}
	var keyval protoreflect.Value
	if err := starToProto(k, m.keyfd, &keyval); err != nil {
		return err
	}
	key := keyval.MapKey()

	val := m.m.NewValue()
	if err := starToProto(k, m.valfd, &val); err != nil {
		return err
	}
	m.m.Set(key, val)
	if m.isMutableType() {
		if !isOwnType(v) {
			v = protoToStar(val, m.valfd)
		}
		m.refs[k] = v
	}
	return nil
}
func (m *Map) String() string {
	buf := new(strings.Builder)
	buf.WriteByte('{')
	if m.m.IsValid() {
		for i, item := range m.Items() {
			if i > 0 {
				buf.WriteString(", ")
			}
			k, v := item[0], item[1]

			buf.WriteString(k.String())
			buf.WriteString(": ")
			buf.WriteString(v.String())
		}
	}
	buf.WriteByte('}')
	return buf.String()
}
func (m *Map) Type() string { return "proto.map" } // TODO
func (m *Map) isMutableType() bool {
	isMutable := m.valfd.Kind() == protoreflect.MessageKind
	if isMutable && m.refs == nil && !m.frozen {
		m.refs = make(map[interface{}]starlark.Value) // lazy init
	}
	return isMutable
}
func (m *Map) Freeze() {
	if !m.frozen {
		m.frozen = true
		for _, v := range m.refs {
			v.Freeze()
		}
	}
}
func (m *Map) Truth() starlark.Bool  { return m.Len() > 0 }
func (m *Map) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: map") }
func (m *Map) checkMutable(verb string) error {
	if m.frozen {
		return fmt.Errorf("cannot %s frozen map", verb)
	}
	if m.itercount > 0 {
		return fmt.Errorf("cannot %s map during iteration", verb)
	}
	return nil
}

type mapAttr func(m *Map) starlark.Value

// methods from starlark/library.go
var mapAttrs = map[string]mapAttr{
	"clear": func(m *Map) starlark.Value { return starext.MakeMethod(m, "clear", m.clear) },
	"get":   func(m *Map) starlark.Value { return starext.MakeMethod(m, "get", m.get) },
	"items": func(m *Map) starlark.Value { return starext.MakeMethod(m, "items", m.items) },
	"keys":  func(m *Map) starlark.Value { return starext.MakeMethod(m, "keys", m.keys) },
	"pop":   func(m *Map) starlark.Value { return starext.MakeMethod(m, "pop", m.pop) },
	//"popitem":    starlark.NewBuiltin("popitem", dict_popitem), // TODO: list based?
	"setdefault": func(m *Map) starlark.Value { return starext.MakeMethod(m, "setdefault", m.setdefault) },
	//"update":     starlark.NewBuiltin("update", dict_update), // TODO: update list.
	"values": func(m *Map) starlark.Value { return starext.MakeMethod(m, "values", m.values) },
}

func (m *Map) Attr(name string) (starlark.Value, error) {
	if a := mapAttrs[name]; a != nil {
		return a(m), nil
	}
	return nil, nil
}
func (m *Map) AttrNames() []string {
	names := make([]string, 0, len(mapAttrs))
	for name := range mapAttrs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (m *Map) clear(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 0); err != nil {
		return nil, err
	}
	if err := m.Clear(); err != nil {
		return nil, err
	}
	return starlark.None, nil
}

func (m *Map) get(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var key, dflt starlark.Value
	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 1, &key, &dflt); err != nil {
		return nil, err
	}
	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 0); err != nil {
		return nil, err
	}
	if v, ok, err := m.Get(key); err != nil {
		return nil, err
	} else if ok {
		return v, nil
	} else if dflt != nil {
		return dflt, nil
	}
	return starlark.None, nil
}

func (m *Map) items(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 0); err != nil {
		return nil, err
	}
	items := m.Items()
	res := make([]starlark.Value, len(items))
	for i, item := range items {
		res[i] = item // convert [2]Value to Value
	}
	return starlark.NewList(res), nil
}

func (m *Map) keys(_ *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 0); err != nil {
		return nil, err
	}
	return starlark.NewList(m.Keys()), nil
}

func (m *Map) pop(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var k, d starlark.Value
	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 1, &k, &d); err != nil {
		return nil, err
	}
	if v, found, err := m.Delete(k); err != nil {
		return nil, err
	} else if found {
		return v, nil
	} else if d != nil {
		return d, nil
	}
	return nil, fmt.Errorf("%s: missing key", fnname)
}

func (m *Map) setdefault(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var key, dflt starlark.Value = nil, starlark.None
	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 1, &key, &dflt); err != nil {
		return nil, err
	}
	if v, ok, err := m.Get(key); err != nil {
		return nil, err
	} else if ok {
		return v, nil
	} else if err := m.SetKey(key, dflt); err != nil {
		return nil, err
	}
	return dflt, nil
}

/*func (m *Map) update(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 1 {
		return nil, fmt.Errorf("update: got %d arguments, want at most 1", len(args))
	}
	// TODO: update
	return starlark.None, nil
}*/

func (m *Map) values(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 0); err != nil {
		return nil, err
	}
	items := m.Items()
	res := make([]starlark.Value, len(items))
	for i, item := range items {
		res[i] = item[1]
	}
	return starlark.NewList(res), nil
}
