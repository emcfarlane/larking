// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package larking

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/textproto"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/genproto/googleapis/api/annotations"
	_ "google.golang.org/genproto/googleapis/api/httpbody"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	_ "google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
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

func (v *variable) String() string {
	return fmt.Sprintf("%#v", v)
}

type variables []*variable

func (p variables) Len() int           { return len(p) }
func (p variables) Less(i, j int) bool { return p[i].name < p[j].name }
func (p variables) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

type path struct {
	segments  map[string]*path   // maps constants to path routes
	variables variables          // sorted array of variables
	methods   map[string]*method // maps http methods to grpc methods
}

func (p *path) String() string {
	var s, sp, sv, sm []string
	for k, pp := range p.segments {
		sp = append(sp, "\""+k+"\":"+pp.String())
	}
	if len(sp) > 0 {
		sort.Strings(sp)
		s = append(s, "segments{"+strings.Join(sp, ",")+"}")
	}

	for _, vv := range p.variables {
		sv = append(sv, "\"{"+vv.name+"}\"->"+vv.next.String())
	}
	if len(sv) > 0 {
		sort.Strings(sv)
		s = append(s, "variables["+strings.Join(sv, ",")+"]")
	}

	for k, mm := range p.methods {
		sm = append(sm, "\""+k+"\":"+mm.String())
	}
	if len(sm) > 0 {
		sort.Strings(sm)
		s = append(s, "methods{"+strings.Join(sm, ",")+"}")
	}
	return "path{" + strings.Join(s, ",") + "}"
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
		methods:  make(map[string]*method),
	}
}

type method struct {
	desc    protoreflect.MethodDescriptor
	body    []protoreflect.FieldDescriptor   // body
	vars    [][]protoreflect.FieldDescriptor // variables on path
	hasBody bool                             // body="*" or body="field.name" or body="" for no body
	resp    []protoreflect.FieldDescriptor   // body=[""|"*"]
	name    string                           // /{ServiceName}/{MethodName}
}

func (m *method) String() string {
	return m.name
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

func (p *path) alive() bool {
	return len(p.methods) != 0 ||
		len(p.variables) != 0 ||
		len(p.segments) != 0
}

// clone deep clones the path tree.
func (p *path) clone() *path {
	pc := newPath()
	if p == nil {
		return pc
	}

	for k, s := range p.segments {
		pc.segments[k] = s.clone()
	}

	pc.variables = make(variables, len(p.variables))
	for i, v := range p.variables {
		pc.variables[i] = &variable{
			name: v.name, // RO
			toks: v.toks, // RO
			next: v.next.clone(),
		}
	}

	for k, m := range p.methods {
		pc.methods[k] = m // RO
	}

	return pc
}

// delRule deletes the HTTP rule to the path.
func (p *path) delRule(name string) bool {
	// dfs
	for k, s := range p.segments {
		if ok := s.delRule(name); ok {
			if !s.alive() {
				delete(p.segments, k)
			}
			return ok
		}
	}

	for i, v := range p.variables {
		if ok := v.next.delRule(name); ok {
			if !v.next.alive() {
				p.variables = append(
					p.variables[:i], p.variables[i+1:]...,
				)
			}
			return ok
		}
	}

	for k, m := range p.methods {
		if m.name == name {
			delete(p.methods, k)
			return true
		}
	}
	return false
}

type parser struct {
	cursor *path
	fields protoreflect.FieldDescriptors
	vars   [][]protoreflect.FieldDescriptor
}

func (p *parser) parseFieldPath(keys tokens, t token) (parseFn, error) {
	if len(keys) == 0 {
		return nil, fmt.Errorf("empty field path: %v %v", keys, t)
	}

	assign := func(vals tokens, t token) (parseFn, error) {
		if t.typ != tokenVariableEnd {
			return nil, fmt.Errorf("unexpected variable end")
		}

		keyVals := keys.vals()
		valVals := vals.vals()
		varLookup := strings.Join(valVals, "")

		fds := fieldPath(p.fields, keyVals...)
		if fds == nil {
			return nil, fmt.Errorf("field not found %v", keys)
		}

		// TODO: validate tokens.
		// TODO: field type checking.

		p.vars = append(p.vars, fds)

		if v, ok := p.cursor.findVariable(varLookup); ok {
			p.cursor = v.next
			return p.parseTemplate, nil
		}

		v := &variable{
			name: varLookup,
			toks: vals,
			next: newPath(),
		}
		p.cursor.variables = append(p.cursor.variables, v)
		sort.Sort(p.cursor.variables)
		p.cursor = v.next

		return p.parseTemplate, nil
	}

	if t.typ == tokenEqual {
		return collect(
			[]tokenType{tokenSlash, tokenStar, tokenStarStar, tokenValue},
			nil,
			assign,
		), nil
	}

	// Default fields are assigned to "*".
	return assign([]token{{
		typ: tokenStar,
		val: "*",
	}}, t)
}

func (p *parser) parseSegments(t token) (parseFn, error) {
	switch t.typ {
	case tokenStar:
		return nil, fmt.Errorf("token %q must be assigned to a variable", t.val)

	case tokenStarStar:
		return nil, fmt.Errorf("token %q must be assigned to a variable", t.val)

	case tokenValue:
		val := "/" + t.val // Prefix for easier matching
		next, ok := p.cursor.segments[val]
		if !ok {
			next = newPath()
			p.cursor.segments[val] = next
		}
		p.cursor = next
		return p.parseTemplate, nil

	case tokenVariableStart:
		return collect(
			[]tokenType{tokenValue},
			[]tokenType{tokenDot},
			p.parseFieldPath,
		), nil

	case tokenEOF:
		return nil, fmt.Errorf("invalid path end %q", t.val)

	default:
		return nil, fmt.Errorf("unexpected %q", t)
	}
}

func (p *parser) parseVerb(t token) (parseFn, error) {
	switch t.typ {
	case tokenValue:
		val := ":" + t.val // Prefix for easier matching
		next, ok := p.cursor.segments[val]
		if !ok {
			next = newPath()
			p.cursor.segments[val] = next
		}
		p.cursor = next
		return p.parseTemplate, nil

	default:
		return nil, fmt.Errorf("verb unexpected %q", t)
	}
}

func (p *parser) parseTemplate(t token) (parseFn, error) {
	switch t.typ {
	case tokenSlash:
		return p.parseSegments, nil
	case tokenVerb:
		return p.parseVerb, nil
	case tokenEOF:
		return nil, nil
	default:
		return nil, fmt.Errorf("template unexpected %q", t)
	}
}

// addRule adds the HTTP rule to the path.
func (p *path) addRule(
	rule *annotations.HttpRule,
	desc protoreflect.MethodDescriptor,
	name string,
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
	case *annotations.HttpRule_Custom:
		verb = v.Custom.Kind
		tmpl = v.Custom.Path
	default:
		return fmt.Errorf("unsupported pattern %v", v)
	}

	msgDesc := desc.Input()
	fieldDescs := msgDesc.Fields()

	// Hold state for the parser.
	pr := &parser{
		cursor: p,
		fields: fieldDescs,
	}
	// Hold state for the lexer.
	l := &lexer{
		parse: pr.parseTemplate,
		input: tmpl,
	}
	if err := lexTemplate(l); err != nil {
		return err
	}

	cursor := pr.cursor
	if _, ok := cursor.methods[verb]; ok {
		return fmt.Errorf("duplicate rule %v", rule)
	}

	m := &method{
		desc: desc,
		vars: pr.vars,
		name: name,
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

	for _, addRule := range rule.AdditionalBindings {
		if len(addRule.AdditionalBindings) != 0 {
			return fmt.Errorf("nested rules") // TODO: errors...
		}

		if err := p.addRule(addRule, desc, name); err != nil {
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

	case protoreflect.MessageKind:
		// Well known JSON scalars are decoded to message types.
		md := fd.Message()
		switch md.FullName() {
		case "google.protobuf.Timestamp":
			var msg timestamppb.Timestamp
			if err := protojson.Unmarshal(raw, &msg); err != nil {
				return param{}, err
			}
			return param{fds, protoreflect.ValueOfMessage(msg.ProtoReflect())}, nil
		case "google.protobuf.Duration":
			var msg durationpb.Duration
			if err := protojson.Unmarshal(raw, &msg); err != nil {
				return param{}, err
			}
			return param{fds, protoreflect.ValueOfMessage(msg.ProtoReflect())}, nil
		case "google.protobuf.BoolValue":
			var msg wrapperspb.BoolValue
			if err := protojson.Unmarshal(raw, &msg); err != nil {
				return param{}, err
			}
			return param{fds, protoreflect.ValueOfMessage(msg.ProtoReflect())}, nil
		case "google.protobuf.Int32Value":
			var msg wrapperspb.Int32Value
			if err := protojson.Unmarshal(raw, &msg); err != nil {
				return param{}, err
			}
			return param{fds, protoreflect.ValueOfMessage(msg.ProtoReflect())}, nil
		case "google.protobuf.Int64Value":
			var msg wrapperspb.Int64Value
			if err := protojson.Unmarshal(raw, &msg); err != nil {
				return param{}, err
			}
			return param{fds, protoreflect.ValueOfMessage(msg.ProtoReflect())}, nil
		case "google.protobuf.UInt32Value":
			var msg wrapperspb.UInt32Value
			if err := protojson.Unmarshal(raw, &msg); err != nil {
				return param{}, err
			}
			return param{fds, protoreflect.ValueOfMessage(msg.ProtoReflect())}, nil
		case "google.protobuf.UInt64Value":
			var msg wrapperspb.UInt64Value
			if err := protojson.Unmarshal(raw, &msg); err != nil {
				return param{}, err
			}
			return param{fds, protoreflect.ValueOfMessage(msg.ProtoReflect())}, nil
		case "google.protobuf.FloatValue":
			var msg wrapperspb.FloatValue
			if err := protojson.Unmarshal(raw, &msg); err != nil {
				return param{}, err
			}
			return param{fds, protoreflect.ValueOfMessage(msg.ProtoReflect())}, nil
		case "google.protobuf.DoubleValue":
			var msg wrapperspb.DoubleValue
			if err := protojson.Unmarshal(raw, &msg); err != nil {
				return param{}, err
			}
			return param{fds, protoreflect.ValueOfMessage(msg.ProtoReflect())}, nil
		case "google.protobuf.BytesValue":
			if n := len(raw); n > 0 && (raw[0] != '"' || raw[n-1] != '"') {
				raw = []byte(strconv.Quote(string(raw)))
			}
			var msg wrapperspb.BytesValue
			if err := protojson.Unmarshal(raw, &msg); err != nil {
				return param{}, err
			}
			return param{fds, protoreflect.ValueOfMessage(msg.ProtoReflect())}, nil
		case "google.protobuf.StringValue":
			if n := len(raw); n > 0 && (raw[0] != '"' || raw[n-1] != '"') {
				raw = []byte(strconv.Quote(string(raw)))
			}
			var msg wrapperspb.StringValue
			if err := protojson.Unmarshal(raw, &msg); err != nil {
				return param{}, err
			}
			return param{fds, protoreflect.ValueOfMessage(msg.ProtoReflect())}, nil

		//case "google.protobuf.FieldMask": // TODO
		//	var msg fieldmaskpb.FieldMask
		//	if err := protojson.Unmarshal(raw, &msg); err != nil {
		//		return param{}, err
		//	}
		//	return param{fds, protoreflect.ValueOfMessage(msg.ProtoReflect())}, nil
		default:
			return param{}, fmt.Errorf("unexpected message type %s", md.FullName())
		}

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
	//fmt.Println("\tlen(v.toks)", len(v.toks))
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
			//fmt.Println("\ttokenSlash", i)

		case tokenStar:
			i = strings.Index(s[i:], "/")
			if i == -1 {
				i = len(s)
			}
			//fmt.Println("\ttokenStar", i)

		case tokenStarStar:
			i = len(s)
			//fmt.Println("\ttokenStarStar", i)

		case tokenValue:
			if !strings.HasPrefix(s[i:], tok.val) {
				return -1
			}
			i += len(tok.val)
			//fmt.Println("\ttokenValue", i)

		default:
			panic(":(")
		}
	}
	return i
}

// Depth first search preferring path segments over variables.
// Variables split the search tree:
//     /path/{variable/*}/to/{end/**} VERB
func (p *path) match(route, method string) (*method, params, error) {
	if len(route) == 0 {
		if m, ok := p.methods[method]; ok {
			return m, nil, nil
		}
		return nil, nil, status.Error(codes.NotFound, "not found")
	}

	j := strings.Index(route[1:], "/") + 1
	if j == 0 {
		j = len(route) // capture end of path
	}
	segment := route[:j]

	//fmt.Println("------------------")
	//fmt.Println("~", segment, "~")

	if next, ok := p.segments[segment]; ok {
		if m, ps, err := next.match(route[j:], method); err == nil {
			return m, ps, err
		}
	}

	for _, v := range p.variables {
		l := v.index(route[1:]) + 1 // bump off /
		if l == 0 {
			continue
		}
		newRoute := route[l:]

		//fmt.Println("variable", l, newRoute)
		m, ps, err := v.next.match(newRoute, method)
		if err != nil {
			continue
		}

		capture := []byte(route[1:l])

		//fmt.Println("capture", len(m.vars), len(ps))
		fds := m.vars[len(m.vars)-len(ps)-1]
		if p, err := parseParam(fds, capture); err != nil {
			return nil, nil, err
		} else {
			ps = append(ps, p)
		}
		return m, ps, err
	}

	return nil, nil, status.Error(codes.NotFound, "not found")
}

const httpHeaderPrefix = "http-"

func newIncomingContext(ctx context.Context, header http.Header) context.Context {
	md := make(metadata.MD, len(header))
	for k, vs := range header {
		md.Set(httpHeaderPrefix+k, vs...)
	}
	return metadata.NewIncomingContext(ctx, md)
}

func setOutgoingHeader(header http.Header, mds ...metadata.MD) {
	for _, md := range mds {
		for k, vs := range md {
			if !strings.HasPrefix(k, httpHeaderPrefix) {
				continue
			}
			k = k[len(httpHeaderPrefix):]
			if len(k) == 0 {
				continue
			}
			header[textproto.CanonicalMIMEHeaderKey(k)] = vs
		}
	}
}

func (m *method) decodeRequestArgs(args proto.Message, r *http.Request) error {
	contentType := r.Header.Get("Content-Type")
	contentEncoding := r.Header.Get("Content-Encoding")

	var body io.ReadCloser
	switch contentEncoding {
	case "gzip":
		var err error
		body, err = gzip.NewReader(r.Body)
		if err != nil {
			return err
		}

	default:
		body = r.Body
	}
	defer body.Close()

	// TODO: mux options.
	b, err := ioutil.ReadAll(io.LimitReader(body, 1024*1024*2))
	if err != nil {
		return err
	}

	cur := args.ProtoReflect()
	for _, fd := range m.body {
		cur = cur.Mutable(fd).Message()
	}
	fullname := cur.Descriptor().FullName()

	msg := cur.Interface()

	switch fullname {
	case "google.api.HttpBody":
		rfl := msg.ProtoReflect()
		fds := rfl.Descriptor().Fields()
		fdContentType := fds.ByName(protoreflect.Name("content_type"))
		fdData := fds.ByName(protoreflect.Name("data"))
		rfl.Set(fdContentType, protoreflect.ValueOfString(contentType))
		rfl.Set(fdData, protoreflect.ValueOfBytes(b))
		// TODO: extensions?

	default:
		// TODO: contentType check?
		if err := protojson.Unmarshal(b, msg); err != nil {
			return err
		}
	}
	return nil
}

func (m *method) encodeResponseReply(
	reply proto.Message, w http.ResponseWriter, r *http.Request,
	header, trailer metadata.MD,
) error {
	//accept := r.Header.Get("Accept")
	acceptEncoding := r.Header.Get("Accept-Encoding")

	if fRsp, ok := w.(http.Flusher); ok {
		defer fRsp.Flush()
	}

	setOutgoingHeader(w.Header(), header, trailer)

	var resp io.Writer
	switch acceptEncoding {
	case "gzip":
		w.Header().Set("Content-Encoding", "gzip")
		gRsp := gzip.NewWriter(w)
		defer gRsp.Close()
		resp = gRsp

	default:
		resp = w
	}

	cur := reply.ProtoReflect()
	for _, fd := range m.resp {
		cur = cur.Mutable(fd).Message()
	}

	msg := cur.Interface()

	switch cur.Descriptor().FullName() {
	case "google.api.HttpBody":
		rfl := msg.ProtoReflect()
		fds := rfl.Descriptor().Fields()
		fdContentType := fds.ByName(protoreflect.Name("content_type"))
		fdData := fds.ByName(protoreflect.Name("data"))
		pContentType := rfl.Get(fdContentType)
		pData := rfl.Get(fdData)

		w.Header().Set("Content-Type", pContentType.String())
		if _, err := io.Copy(resp, bytes.NewReader(pData.Bytes())); err != nil {
			return err
		}

	default:
		// TODO: contentType check?
		b, err := protojson.Marshal(msg)
		if err != nil {
			return err
		}

		w.Header().Set("Content-Type", "application/json")
		if _, err := io.Copy(resp, bytes.NewReader(b)); err != nil {
			return err
		}
	}

	return nil
}

func (m *Mux) proxyHTTP(w http.ResponseWriter, r *http.Request) error {
	if !strings.HasPrefix(r.URL.Path, "/") {
		r.URL.Path = "/" + r.URL.Path
	}

	//d, err := httputil.DumpRequest(r, true)
	//if err != nil {
	//	return err
	//}
	//fmt.Println(string(d))

	//for k, v := range r.Header {
	//	fmt.Println(k, v)
	//}

	s := m.loadState()

	method, params, err := s.path.match(r.URL.Path, r.Method)
	if err != nil {
		return err
	}

	mc, err := s.pickMethodConn(method.name)
	if err != nil {
		return err
	}

	// TODO: fix the body marshalling
	argsDesc := method.desc.Input()
	replyDesc := method.desc.Output()
	//fmt.Printf("\n%s -> %s\n", argsDesc.FullName(), replyDesc.FullName())

	args := dynamicpb.NewMessage(argsDesc)
	reply := dynamicpb.NewMessage(replyDesc)

	if method.hasBody {
		// TODO: handler should decide what to select on.
		if err := method.decodeRequestArgs(args, r); err != nil {
			return err
		}
	}

	queryParams, err := method.parseQueryParams(r.URL.Query())
	if err != nil {
		return err
	}
	params = append(params, queryParams...)
	//fmt.Println("queryParams", len(queryParams), queryParams)

	if err := params.set(args); err != nil {
		return err
	}

	ctx := newIncomingContext(r.Context(), r.Header)

	var header, trailer metadata.MD
	if err := mc.cc.Invoke(
		ctx,
		method.name,
		args, reply,
		grpc.Header(&header),
		grpc.Trailer(&trailer),
	); err != nil {
		return err
	}

	return method.encodeResponseReply(reply, w, r, header, trailer)
}

func encError(w http.ResponseWriter, err error) {
	s, _ := status.FromError(err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(HTTPStatusCode(s.Code()))

	b, err := protojson.Marshal(s.Proto())
	if err != nil {
		panic(err) // ...
	}
	w.Write(b) //nolint
}

func (m *Mux) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if err := m.proxyHTTP(w, r); err != nil {
		encError(w, err)
	}
}
