package graphpb

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	_ "google.golang.org/protobuf/types/descriptorpb"

	"github.com/afking/graphpb/google.golang.org/genproto/googleapis/api/annotations"
	//_ "github.com/afking/graphpb/google.golang.org/genproto/googleapis/api/annotations"
	_ "github.com/afking/graphpb/google.golang.org/genproto/googleapis/api/httpbody"
)

type Handler struct {
	path          *path
	unmarshalOpts protojson.UnmarshalOptions
	marshalOpts   protojson.MarshalOptions
}

// getExtensionHTTP
func getExtensionHTTP(m proto.Message) *annotations.HttpRule {
	return proto.GetExtension(m, annotations.E_Http).(*annotations.HttpRule)
}

type variable struct {
	name string
	fds  []protoreflect.FieldDescriptor // name.to.field
	exp  *regexp.Regexp                 // expr/**
	next *path
}

type path struct {
	segments  map[string]*path     // maps constants to path routes
	variables map[string]*variable // maps key=vale names to path variables
	methods   map[string]*method   // maps http methods to grpc methods
}

func newPath() *path {
	return &path{
		segments:  make(map[string]*path),
		variables: make(map[string]*variable),
		methods:   make(map[string]*method),
	}
}

type method struct {
	desc protoreflect.MethodDescriptor
	//url      *url.URL // /<service.Service>/<Method>
	body     []protoreflect.FieldDescriptor
	bodyStar bool                           // body="*" no params
	resp     []protoreflect.FieldDescriptor // TODO: this can only be single?
	//respStar bool                           // body=[""|"*"]
	invoke invoker
}

func fieldPath(fieldDescs protoreflect.FieldDescriptors, names ...string) []protoreflect.FieldDescriptor {
	fds := make([]protoreflect.FieldDescriptor, len(names))
	for i, name := range names {
		fd := fieldDescs.ByJSONName(name)
		if fd == nil {
			fd = fieldDescs.ByName(protoreflect.Name(name))
		}
		if fd == nil {
			return nil
		}

		fds[i] = fd

		// advance
		if i != len(fds)-1 {
			msgDesc := fd.Message()
			if msgDesc == nil {
				return nil
			}
			fieldDescs = msgDesc.Fields()
		}
	}
	return fds
}

type invoker func(ctx context.Context, args, reply proto.Message) error

func (p *path) parseRule(
	rule *annotations.HttpRule,
	desc protoreflect.MethodDescriptor,
	invoke invoker,
) error {
	var tmpl, verb string
	switch v := rule.Pattern.(type) {
	case *annotations.HttpRule_Get:
		verb = http.MethodGet
		tmpl = v.Get
	case *annotations.HttpRule_Put:
		verb = http.MethodPut
		tmpl = v.Put
	case *annotations.HttpRule_Post:
		verb = http.MethodPost
		tmpl = v.Post
	case *annotations.HttpRule_Delete:
		verb = http.MethodDelete
		tmpl = v.Delete
	case *annotations.HttpRule_Patch:
		verb = http.MethodPatch
		tmpl = v.Patch
	default: // TODO: custom  method support
		return fmt.Errorf("unsupported pattern %v", v)
	}

	l := &lexer{
		state: lexSegment,
		input: tmpl,
	}

	msgDesc := desc.Input()
	fieldDescs := msgDesc.Fields()
	cursor := p // cursor

	var t token
	for t = l.token(); t.isEnd(); t = l.token() {
		//fmt.Println("token", t)

		switch t.typ {
		case tokenSlash:
			continue

		case tokenValue:
			val := "/" + t.val // Prefix for easier matching
			next, ok := cursor.segments[val]
			if !ok {
				next = newPath()
				cursor.segments[val] = next
			}
			cursor = next

		case tokenVariableStart:

			keys, tokNext := l.collect([]tokenType{
				tokenValue,
			}, []tokenType{
				tokenDot,
			})

			var vals []token
			if tokNext.typ == tokenEqual {
				vals, tokNext = l.collect([]tokenType{
					tokenSlash,
					tokenStar,
					tokenStarStar,
					tokenValue,
				}, []tokenType{})
			} else {
				vals = []token{token{
					typ: tokenStar,
					val: "*",
				}} // default
			}

			if tokNext.typ != tokenVariableEnd {
				return fmt.Errorf("unexpected token %+v", tokNext)
			}

			keyVals := tokens(keys).vals()
			valVals := tokens(vals).vals()
			varLookup := strings.Join(keyVals, ".") + "=" +
				strings.Join(valVals, "")

			v := cursor.variables[varLookup]
			if v != nil {
				cursor = v.next
				break
			}

			fds := fieldPath(fieldDescs, keyVals...)
			if fds == nil {
				return fmt.Errorf("field not found %v", keys)
			}

			// Some dodgy regexp conversions
			exp := "^\\/"
			for _, val := range vals {
				switch val.typ {
				case tokenSlash:
					exp += "\\/"
				case tokenStar:
					exp += "[a-zA-Z0-9]+"
				case tokenStarStar:
					exp += "[a-zA-Z0-9\\/]+"
				case tokenValue:
					exp += val.val
				default:
					return fmt.Errorf("regexp unhandled  %v", val)
				}
			}

			r, err := regexp.Compile(exp)
			if err != nil {
				return err
			}

			v = &variable{
				fds:  fds,
				exp:  r,
				next: newPath(),
			}
			cursor.variables[varLookup] = v
			cursor = v.next
		}

	}

	if _, ok := cursor.methods[verb]; ok {
		return fmt.Errorf("duplicate rule %v", rule)
	}

	m := &method{
		desc: desc,
		//url:  grpcURL,
		invoke: invoke,
	}
	switch rule.Body {
	case "*":
		m.bodyStar = true
	case "":
		m.bodyStar = false
	default:
		m.body = fieldPath(fieldDescs, strings.Split(rule.Body, ".")...)
		if m.body == nil {
			return fmt.Errorf("body field error %v", rule.Body)
		}
	}

	switch rule.ResponseBody {
	case "":
	default:
		m.resp = fieldPath(fieldDescs, strings.Split(rule.Body, ".")...)
		if m.resp == nil {
			return fmt.Errorf("response body field error %v", rule.ResponseBody)
		}
	}

	cursor.methods[verb] = m // register method
	fmt.Println("Registered", verb, tmpl)

	for _, addRule := range rule.AdditionalBindings {
		// TODO: nested value check?
		if err := p.parseRule(addRule, desc, invoke); err != nil {
			return err
		}
	}

	return nil
}

type param struct {
	fds []protoreflect.FieldDescriptor
	val protoreflect.Value
	//val string // raw value
	//val []byte
}

func parseParam(fds []protoreflect.FieldDescriptor, raw []byte) (param, error) {
	if len(fds) == 0 {
		return param{}, fmt.Errorf("zero field")
	}
	fd := fds[len(fds)-1]

	switch kind := fd.Kind(); kind {
	case protoreflect.BoolKind:
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return param{}, err
		}
		return param{fds, protoreflect.ValueOfBool(b)}, nil

	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		var x int32
		if err := json.Unmarshal(raw, &x); err != nil {
			return param{}, err
		}
		return param{fds, protoreflect.ValueOfInt32(x)}, nil

	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		var x int64
		if err := json.Unmarshal(raw, &x); err != nil {
			return param{}, err
		}
		return param{fds, protoreflect.ValueOfInt64(x)}, nil

	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		var x uint32
		if err := json.Unmarshal(raw, &x); err != nil {
			return param{}, err
		}
		return param{fds, protoreflect.ValueOfUint32(x)}, nil

	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		var x uint64
		if err := json.Unmarshal(raw, &x); err != nil {
			return param{}, err
		}
		return param{fds, protoreflect.ValueOfUint64(x)}, nil

	case protoreflect.FloatKind:
		var x float32
		if err := json.Unmarshal(raw, &x); err != nil {
			return param{}, err
		}
		return param{fds, protoreflect.ValueOfFloat32(x)}, nil

	case protoreflect.DoubleKind:
		var x float64
		if err := json.Unmarshal(raw, &x); err != nil {
			return param{}, err
		}
		return param{fds, protoreflect.ValueOfFloat64(x)}, nil

	case protoreflect.StringKind:
		return param{fds, protoreflect.ValueOfString(string(raw))}, nil

	case protoreflect.BytesKind:
		enc := base64.StdEncoding
		if bytes.ContainsAny(raw, "-_") {
			enc = base64.URLEncoding
		}
		if len(raw)%4 != 0 {
			enc = enc.WithPadding(base64.NoPadding)
		}

		dst := make([]byte, enc.DecodedLen(len(raw)))
		n, err := enc.Decode(dst, raw)
		if err != nil {
			return param{}, err
		}
		return param{fds, protoreflect.ValueOfBytes(dst[:n])}, nil

	case protoreflect.EnumKind:
		var x int32
		if err := json.Unmarshal(raw, &x); err == nil {
			return param{fds, protoreflect.ValueOfEnum(protoreflect.EnumNumber(x))}, nil
		}

		s := string(raw)
		if isNullValue(fd) && s == "null" {
			return param{fds, protoreflect.ValueOfEnum(0)}, nil
		}

		enumVal := fd.Enum().Values().ByName(protoreflect.Name(s))
		if enumVal == nil {
			return param{}, fmt.Errorf("unexpected enum %s", raw)
		}
		return param{fds, protoreflect.ValueOfEnum(enumVal.Number())}, nil

	default:
		return param{}, fmt.Errorf("unknown param type %s", kind)

	}
}

func isNullValue(fd protoreflect.FieldDescriptor) bool {
	ed := fd.Enum()
	return ed != nil && ed.FullName() == "google.protobuf.NullValue"
}

type params []param

func (ps params) set(m proto.Message) error {
	for _, p := range ps {
		cur := m.ProtoReflect()
		for i, fd := range p.fds {
			if len(p.fds)-1 == i {
				cur.Set(fd, p.val)
				break
			}

			// TODO: more types?
			cur = cur.Mutable(fd).Message()
			// IsList()
			// IsMap()
		}

	}
	return nil
}

func (m *method) parseQueryParams(values url.Values) (params, error) {
	msgDesc := m.desc.Input()
	fieldDescs := msgDesc.Fields()

	var ps params
	for key, vs := range values {
		fds := fieldPath(fieldDescs, strings.Split(key, ".")...)
		if fds == nil {
			continue
		}

		for _, v := range vs {
			p, err := parseParam(fds, []byte(v))
			if err != nil {
				return nil, err
			}
			ps = append(ps, p)
		}
	}
	return ps, nil
}

//func (p *path) match(s, method string) (*method, []*param, error) {
func (p *path) match(s, method string) (*method, params, error) {
	// /some/request/path VERB
	// variables can eat multiple

	path := p
	var params params = []param{}

	for i := 0; i < len(s); {
		j := strings.Index(s[i+1:], "/")
		if j == -1 {
			j = len(s) // capture end of path
		} else {
			j += i + 1
		}

		seg := s[i:j]
		fmt.Println(seg, path.segments)
		if nextPath, ok := path.segments[seg]; ok {
			path = nextPath
			i = j
			fmt.Println("segment", seg)
			continue
		}

		var matched *variable
		fmt.Println("vars", path.variables)
		var k int // greatest length match
		for _, v := range path.variables {
			sub := s[i:]
			loc := v.exp.FindStringIndex(sub)
			if loc != nil && loc[1] > k {
				matched = v
				k = loc[1]
			}

		}
		if matched != nil {
			path = matched.next

			// capture path param
			raw := []byte(s[i+1 : i+k]) // TODO...
			p, err := parseParam(matched.fds, raw)
			if err != nil {
				return nil, nil, err
			}
			params = append(params, p)

			i += k
			continue
		}

		return nil, nil, fmt.Errorf("404")
	}

	m, ok := path.methods[method]
	if !ok {
		return nil, nil, fmt.Errorf("405")
	}
	fmt.Println("FOUND")
	return m, params, nil
}
