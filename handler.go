package gateway

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	"github.com/afking/gateway/google.golang.org/genproto/googleapis/api/annotations"
	//_ "github.com/afking/gateway/google.golang.org/genproto/googleapis/api/annotations"
	_ "github.com/afking/gateway/google.golang.org/genproto/googleapis/api/httpbody"
	_ "google.golang.org/protobuf/types/descriptorpb"
)

type Handler struct {
	path          *path
	srv           *grpc.Server
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
	desc     protoreflect.MethodDescriptor
	url      *url.URL
	body     []protoreflect.FieldDescriptor
	bodyStar bool // body="*" no params
	resp     []protoreflect.FieldDescriptor
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

func parseRule(
	parent *path,
	rule *annotations.HttpRule,
	desc protoreflect.MethodDescriptor,
	grpcURL *url.URL,
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
	}

	l := &lexer{
		state: lexSegment,
		input: tmpl,
	}

	msgDesc := desc.Input()
	fieldDescs := msgDesc.Fields()
	cursor := parent // cursor

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
		url:  grpcURL,
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
	fmt.Println("Registered", verb, tmpl, "->", m.url)

	for _, addRule := range rule.AdditionalBindings {
		// TODO: nested value check?
		if err := parseRule(parent, addRule, desc, grpcURL); err != nil {
			return err
		}
	}

	return nil
}

func NewHandler(gs *grpc.Server) (*Handler, error) {
	h := &Handler{
		path: newPath(),
		srv:  gs,
	}

	for name, info := range gs.GetServiceInfo() {
		file, ok := info.Metadata.(string)
		if !ok {
			return nil, fmt.Errorf("service %q has unexpected metadata %v", name, info.Metadata)
		}

		fd, err := protoregistry.GlobalFiles.FindFileByPath(file)
		if err != nil {
			return nil, err
		}

		sds := fd.Services()
		for i := 0; i < sds.Len(); i++ {
			sd := sds.Get(i)
			if string(sd.FullName()) != name {
				continue
			}
			fmt.Println("sd.FuleName()", sd.FullName())

			mds := sd.Methods()
			for j := 0; j < mds.Len(); j++ {
				md := mds.Get(j)

				opts := md.Options()
				if opts == nil {
					continue
				}

				u, err := url.Parse(fmt.Sprintf(
					"/%s/%s", name, md.Name(),
				))
				if err != nil {
					return nil, err
				}

				rule := getExtensionHTTP(opts)
				fmt.Printf("%T %+v\n", rule, rule)

				//fmt.Println("parseRule()", u, rule)
				if err := parseRule(h.path, rule, md, u); err != nil {
					return nil, err
				}
			}
		}
	}

	fmt.Printf("%+v\n", h.path)

	return h, nil
}

const contentType = "application/grpc+" + Name

// http.ResponseWriter + unmarshaler
type transformer struct {
	//body io.ReadCloser
	b   []byte
	w   http.ResponseWriter
	f   http.Flusher
	hdr http.Header
	m   *method
	ps  []*param

	buf bytes.Buffer
}

func (t *transformer) Flush() {
	fmt.Println("FLUSH")
	t.f.Flush()
}

func (t *transformer) Header() http.Header {
	return t.hdr
}

func (t *transformer) Write(b []byte) (n int, err error) {
	//return 0, fmt.Errorf("fudge")
	//fmt.Println("WRITE", b, "the")
	n, err = t.buf.Write(b)
	if err != nil {
		return
	}

	if t.buf.Len() <= 5 {
		return
	}

	buf := t.buf.Bytes()
	nKey := int(binary.BigEndian.Uint32(buf[1:5]))
	if 5+nKey > t.buf.Len() {
		return
	}
	key := t.buf.Next(5 + nKey)[5:]
	//fmt.Println("key", key)

	v, err := globalCodec.decode(key)
	if err != nil {
		return n, err
	}

	return n, t.marshal(v)
}

func (t *transformer) WriteHeader(statusCode int) {
	// write some headers
	hdrs := t.w.Header()
	for key, values := range t.hdr {
		// TODO: filtering options
		hdrs[key] = values
	}
	t.w.WriteHeader(statusCode)

	fmt.Println("WRITE STATUS", statusCode)
}

func toMessage(v interface{}) (proto.Message, error) {
	m, ok := v.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("expected proto.Message received %T", v)
	}
	return m, nil
}

func (t *transformer) marshal(v interface{}) error {
	fmt.Printf("marshal %T\n", v)
	m, err := toMessage(v)
	if err != nil {
		return err
	}

	// TODO: response body fields
	// TODO: json options

	mBuf, err := protojson.Marshal(m)
	if err != nil {
		return err
	}
	_, err = t.w.Write(mBuf)
	fmt.Println("WRITING", string(mBuf))
	return err
}

func (t *transformer) unmarshal(v interface{}) error {
	fmt.Printf("unmarshal %T\n", v)
	m, err := toMessage(v)
	if err != nil {
		return err
	}

	// TODO: body fields
	// TODO: json options
	if len(t.b) > 0 {
		if err := protojson.Unmarshal(t.b, m); err != nil {
			fmt.Println("here?", string(t.b), err)
			return err
		}
	}

	if len(t.ps) > 0 {
		msg := m.ProtoReflect()
		for _, p := range t.ps {
			cur := msg
			for i, fd := range p.fds {
				if len(p.fds)-1 == i {
					cur.Set(fd, p.val)
				} else {
					// TODO: more types
					cur = cur.Mutable(fd).Message()
				}
			}

		}
	}

	return nil
}

type param struct {
	fds []protoreflect.FieldDescriptor
	val protoreflect.Value
	//val string // raw value
}

func parseParam(fds []protoreflect.FieldDescriptor, raw []byte) (*param, error) {
	if len(fds) == 0 {
		return nil, fmt.Errorf("zero field")
	}

	fd := fds[len(fds)-1]

	var val protoreflect.Value
	switch fd.Kind() {
	case protoreflect.BoolKind:
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return nil, err
		}
		val = protoreflect.ValueOf(b)
	case protoreflect.StringKind:
		val = protoreflect.ValueOf(string(raw))
	case protoreflect.BytesKind:
		val = protoreflect.ValueOf(raw)

	// TODO: extend
	default:
		return nil, fmt.Errorf("handle desc %v", fd)
	}
	return &param{fds, val}, nil
}

func (h *Handler) match(r *http.Request) (*method, []*param, error) {
	s := r.URL.Path
	if !strings.HasPrefix(s, "/") {
		s = "/" + s
		r.URL.Path = s
	}

	// /some/request/path
	// variables can eat multiple

	path := h.path
	params := []*param{}

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

	m, ok := path.methods[r.Method]
	if !ok {
		return nil, nil, fmt.Errorf("405")
	}
	fmt.Println("FOUND")
	return m, params, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	// match handler to URL

	m, ps, err := h.match(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	//fmt.Println("METHOD", m)

	// TODO: query params

	hdr := make(http.Header)
	hdr.Set("content-type", contentType)
	trl := make(http.Header)

	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "requires a ResponseWriter supporting http.Flusher", http.StatusInternalServerError)
		return
	}

	// TODO: sanitise
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := r.Body.Close(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	t := &transformer{
		b:   body,
		w:   w,
		f:   f,
		hdr: make(http.Header),
		m:   m,
		ps:  ps,
	}

	b := globalCodec.encode(t)
	buf := make([]byte, 5+len(b))
	binary.BigEndian.PutUint32(buf[1:5], uint32(len(b)))
	copy(buf[5:], b)
	gb := bytes.NewReader(buf)
	fmt.Printf("buf %+v %v %v\n", buf, len(buf), len(b))

	// mutate request to satisfy gRPC transport handling...
	r.Method = http.MethodPost
	r.URL = m.url
	r.Proto = "HTTP/2"
	r.ProtoMajor = 2
	r.ProtoMinor = 0
	r.Header = hdr
	r.Body = ioutil.NopCloser(gb)
	r.ContentLength = int64(len(buf))
	r.Trailer = trl

	h.srv.ServeHTTP(t, r)
	fmt.Println("END")
}
