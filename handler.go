package graphpb

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	_ "google.golang.org/protobuf/types/descriptorpb"

	"github.com/afking/graphpb/grpc/codes"
	"github.com/afking/graphpb/grpc/status"

	"github.com/afking/graphpb/google.golang.org/genproto/googleapis/api/annotations"
	_ "github.com/afking/graphpb/google.golang.org/genproto/googleapis/api/httpbody"
)

// getExtensionHTTP
func getExtensionHTTP(m proto.Message) *annotations.HttpRule {
	return proto.GetExtension(m, annotations.E_Http).(*annotations.HttpRule)
}

type variable struct {
	name string  // path.to.field=segment/*/***
	toks []token // segment/*/**
	next *path
}

type variables []*variable

func (p variables) Len() int           { return len(p) }
func (p variables) Less(i, j int) bool { return p[i].name < p[j].name }
func (p variables) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

type path struct {
	segments  map[string]*path // maps constants to path routes
	variables variables        // sorted array of variables
	//variables map[string]*variable // maps key=vale names to path variables
	methods map[string]*method // maps http methods to grpc methods
}

func (p *path) findVariable(name string) (*variable, bool) {
	for _, v := range p.variables {
		if v.name == name {
			return v, true
		}
	}
	return nil, false
}

func newPath() *path {
	return &path{
		segments: make(map[string]*path),
		//variables: make(map[string]*variable),
		methods: make(map[string]*method),
	}
}

type method struct {
	desc    protoreflect.MethodDescriptor
	body    []protoreflect.FieldDescriptor   // body
	vars    [][]protoreflect.FieldDescriptor // variables on path
	hasBody bool                             // body="*" or body="field.name" or body="" for no body
	resp    []protoreflect.FieldDescriptor   // body=[""|"*"]
	invoke  invoker
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
	var vars [][]protoreflect.FieldDescriptor

	var t token
	for t = l.token(); !t.isEnd(); t = l.token() {
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

			var vals tokens
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
				// TODO: better error reporting.
				return fmt.Errorf("unexpected token %+v", tokNext)
			}

			keyVals := keys.vals()
			valVals := vals.vals()
			varLookup := strings.Join(keyVals, ".") + "=" +
				strings.Join(valVals, "")

			fds := fieldPath(fieldDescs, keyVals...)
			if fds == nil {
				return fmt.Errorf("field not found %v", keys)
			}

			// TODO: validate tokens.
			// TODO: field type checking.

			vars = append(vars, fds)

			if v, ok := p.findVariable(varLookup); ok {
				cursor = v.next
				continue
			}

			v := &variable{
				name: varLookup,
				toks: vals,
				next: newPath(),
			}
			cursor.variables = append(cursor.variables, v)
			sort.Sort(cursor.variables)
			cursor = v.next
		}

	}

	if _, ok := cursor.methods[verb]; ok {
		return fmt.Errorf("duplicate rule %v", rule)
	}

	m := &method{
		desc:   desc,
		vars:   vars,
		invoke: invoke,
	}
	switch rule.Body {
	case "*":
		m.hasBody = true
	case "":
		m.hasBody = false
	default:
		m.body = fieldPath(fieldDescs, strings.Split(rule.Body, ".")...)
		if m.body == nil {
			return fmt.Errorf("body field error %v", rule.Body)
		}
		m.hasBody = true
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
		if len(addRule.AdditionalBindings) != 0 {
			return fmt.Errorf("nested rules") // TODO: errors...
		}

		if err := p.parseRule(addRule, desc, invoke); err != nil {
			return err
		}
	}

	return nil
}

type param struct {
	fds []protoreflect.FieldDescriptor
	val protoreflect.Value
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

func (v *variable) index(s string) int {
	var i int
	fmt.Println("\tlen(v.toks)", len(v.toks))
	for _, tok := range v.toks {
		if i == len(s) {
			return -1
		}

		switch tok.typ {
		case tokenSlash:
			if !strings.HasPrefix(s[i:], "/") {
				return -1
			}
			i += 1
			fmt.Println("\ttokenSlash", i)

		case tokenStar:
			i = strings.Index(s[i:], "/")
			if i == -1 {
				i = len(s)
			}
			fmt.Println("\ttokenStar", i)

		case tokenStarStar:
			i = len(s)
			fmt.Println("\ttokenStarStar", i)

		case tokenValue:
			if !strings.HasPrefix(s[i:], tok.val) {
				return -1
			}
			i += len(tok.val)
			fmt.Println("\ttokenValue", i)

		default:
			panic(":(")
		}
	}
	return i
}

func (p *path) match(s, method string) (*method, params, error) {
	fmt.Println("SEARCHING FOR", s)
	// /some/request/path VERB
	// variables can eat multiple

	// Depth first search preferring path segments over variables.
	type node struct {
		i int // segment index
		//path   *path // path cursor
		variable *variable //

		captured bool
	}
	var stack []node
	var captures []string

	path := p

searchLoop:
	for i := 0; i < len(s); {
		j := strings.Index(s[i+1:], "/")
		if j == -1 {
			j = len(s) // capture end of path
		} else {
			j += i + 1
		}

		segment := s[i:j]
		fmt.Println("------------------")
		fmt.Println(segment, path.segments)

		// Push path variables to stack.
		for _, v := range path.variables {
			stack = append(stack, node{
				i, v, false,
			})
		}
		//if len(path.variables) != 0 {
		//	stack = append(stack, node{i, j, path, 0})
		//}

		if nextPath, ok := path.segments[segment]; ok {
			path = nextPath
			i = j
			fmt.Println("segment", segment)

			continue
		}
		fmt.Println("segment fault", segment, len(stack))

		for k := len(stack) - 1; k >= 0; k-- {
			n := stack[k]
			stack = stack[:k] // pop
			fmt.Println("len(stack)", len(stack))

			// pop
			if n.captured {
				fmt.Println("pop", n.variable.name, captures[len(captures)-1])
				captures = captures[:len(captures)-1]
				continue
			}

			i = n.i
			l := n.variable.index(s[i+1:]) // check
			if l == -1 {
				fmt.Println("pop", n.variable.name, "<nil>")
				continue
			}
			j = i + l + 1

			// method check
			if j == len(s) {
				if _, ok := n.variable.next.methods[method]; !ok {
					fmt.Println("skipping on method", method)
					continue
				}
			}

			// push
			fmt.Println("indexing", i, j, s, len(s))
			raw := s[i+1 : j]
			fmt.Println("push", n.variable.name, raw)
			n.captured = true
			captures = append(captures, raw)
			path = n.variable.next //
			i = j
			continue searchLoop
		}

		return nil, nil, status.Error(codes.NotFound, "not found")
	}

	m, ok := path.methods[method]
	if !ok {
		return nil, nil, status.Error(codes.NotFound, "not found")
	}

	if len(m.vars) != len(captures) {
		panic(fmt.Sprintf("wft %d %d", len(m.vars), len(captures)))
	}

	params := make(params, len(m.vars))
	for i, fds := range m.vars {
		p, err := parseParam(fds, []byte(captures[i]))
		if err != nil {
			return nil, nil, err
		}
		params[i] = p
	}

	return m, params, nil
}
