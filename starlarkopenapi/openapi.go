// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlarkopenapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/emcfarlane/larking/starext"
	"github.com/emcfarlane/larking/starlarkstruct"
	"github.com/emcfarlane/larking/starlarkthread"
	"github.com/go-openapi/spec"
	"github.com/iancoleman/strcase"
	starlarkjson "go.starlark.net/lib/json"
	starlarktime "go.starlark.net/lib/time"
	"go.starlark.net/starlark"
	"gocloud.dev/runtimevar"
)

func NewModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "openapi",
		Members: starlark.StringDict{
			"open": starext.MakeBuiltin("openapi.open", Open),
		},
	}
}

type Client struct {
	// service encoding...
	name     string
	variable *runtimevar.Variable
	client   *http.Client

	val  []byte // snapshot.Value
	doc  *spec.Swagger
	svcs map[string]*Service //starlark.Value
}

func Open(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		addr string
		name string
	)
	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 1, &addr, &name); err != nil {
		return nil, err
	}

	ctx := starlarkthread.Context(thread)

	variable, err := runtimevar.OpenVariable(ctx, name)
	if err != nil {
		return nil, err
	}

	c := &Client{
		name:     name,
		variable: variable,
	}
	if _, err := c.load(ctx); err != nil {
		variable.Close() //nolint
		return nil, err
	}
	if err := starlarkthread.AddResource(thread, c); err != nil {
		variable.Close() //nolint
		return nil, err
	}
	return c, nil
}

func (c *Client) getClient() *http.Client {
	if c.client != nil {
		return c.client
	}
	return http.DefaultClient
}

func toSnakeCase(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return r
		}
		// ignore variables
		if r == '{' || r == '}' {
			return -1
		}
		return '_'
	}, s)
	s = strcase.ToSnake(s)
	s = strings.Trim(s, "_")
	return s
}

func (c *Client) load(ctx context.Context) (*spec.Swagger, error) {
	snap, err := c.variable.Latest(ctx)
	if err != nil {
		return nil, err
	}

	var b []byte
	switch v := snap.Value.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return nil, fmt.Errorf("unhandled type: %v", v)
	}

	var doc spec.Swagger
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	c.val = b
	c.doc = &doc

	if err := spec.ExpandSpec(&doc, &spec.ExpandOptions{}); err != nil {
		return nil, err
	}

	// build attrs
	if doc.Paths == nil {
		return &doc, nil
	}
	//attrs := make(map[string]*Service)
	//attrNames := make([]string, 0, len(doc.Tags))
	//tagNames := make(map[string]string)
	services := make(map[string]*Service)

	for path, item := range doc.Paths.Paths {
		key := toSnakeCase(path)

		var count int
		addMethod := func(op *spec.Operation, method string) {
			count++
			var svcNames []string
			for _, tag := range op.Tags {
				svcNames = append(svcNames, strcase.ToSnake(tag))
			}
			if len(svcNames) == 0 {
				svcNames = append(svcNames, key)
			}

			mdName := key
			if id := op.ID; id != "" {
				mdName = strcase.ToSnake(id)
			}

			m := &Method{
				c:    c,
				name: mdName,
				path: path,
				op:   op,
				//params: item.Parameters,
				method: method,
			}
			//fmt.Println("name", mdName)
			//fmt.Println("name", path)
			//fmt.Println("op", op)
			//fmt.Println()

			for _, svcName := range svcNames {
				svc, ok := services[svcName]
				if !ok {
					svc = &Service{
						name:    svcName,
						methods: make(map[string]*Method),
					}
					services[svcName] = svc
				}
				svc.methods[mdName] = m
			}
		}

		if v := item.Get; v != nil {
			addMethod(v, http.MethodGet)
		}
		if v := item.Put; v != nil {
			addMethod(v, http.MethodPut)
		}
		if v := item.Post; v != nil {
			addMethod(v, http.MethodPost)
		}
		if v := item.Delete; v != nil {
			addMethod(v, http.MethodDelete)
		}
		if v := item.Options; v != nil {
			addMethod(v, http.MethodOptions)
		}
		if v := item.Head; v != nil {
			addMethod(v, http.MethodHead)
		}
		if v := item.Patch; v != nil {
			addMethod(v, http.MethodPatch)
		}

		if count == 0 {
			return nil, fmt.Errorf("missing operations for path: %s", path)
		}
	}

	c.svcs = services
	return &doc, nil
}

func (c *Client) makeURL(urlPath string, urlQuery url.Values) url.URL {
	scheme := "http"
	if x := c.doc.Schemes; len(x) > 0 {
		scheme = x[0]
	}
	return url.URL{
		Scheme:   scheme,
		Host:     c.doc.Host,
		Path:     path.Join(c.doc.BasePath, urlPath),
		RawQuery: urlQuery.Encode(),
	}
}

func (c *Client) String() string        { return fmt.Sprintf("<client %q>", c.name) }
func (c *Client) Type() string          { return "openapi.client" }
func (c *Client) Freeze()               {} // immutable?
func (c *Client) Truth() starlark.Bool  { return c.variable.CheckHealth() == nil }
func (c *Client) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: %s", c.Type()) }
func (c *Client) Close() error {
	return c.variable.Close()
}

func (c *Client) Attr(name string) (starlark.Value, error) {
	if s, ok := c.svcs[name]; ok {
		return s, nil
	}
	if name == "schema" {
		return starlark.String(string(c.val)), nil
	}
	return nil, nil
}
func (c *Client) AttrNames() []string {
	names := make([]string, 0, len(c.svcs))
	for name := range c.svcs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

type Service struct {
	name    string
	methods map[string]*Method
}

func (s *Service) String() string        { return fmt.Sprintf("<service %q>", s.name) }
func (s *Service) Type() string          { return "openapi.service" }
func (s *Service) Freeze()               {} // immutable?
func (s *Service) Truth() starlark.Bool  { return s.name != "" }
func (s *Service) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: %s", s.Type()) }
func (s *Service) Attr(name string) (starlark.Value, error) {
	if m, ok := s.methods[name]; ok {
		return m, nil
	}
	return nil, nil
}
func (s *Service) AttrNames() []string {
	names := make([]string, 0, len(s.methods))
	for name := range s.methods {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

type Method struct {
	c *Client

	name string
	path string
	op   *spec.Operation
	//params []spec.Parameter
	method string
}

func (m *Method) String() string        { return fmt.Sprintf("<method %q>", m.name) }
func (m *Method) Type() string          { return "openapi.method" }
func (m *Method) Freeze()               {} // immutable?
func (m *Method) Truth() starlark.Bool  { return m.name != "" }
func (m *Method) Hash() (uint32, error) { return starlark.String(m.path).Hash() }

var (
	starlarkJSONEncode = starlarkjson.Module.Members["encode"].(*starlark.Builtin)
	starlarkJSONDecode = starlarkjson.Module.Members["decode"].(*starlark.Builtin)
)

func (m *Method) Name() string { return m.name }
func (m *Method) CallInternal(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := starlarkthread.Context(thread)
	hasArgs := len(args) > 0
	//hasKwargs := len(kwargs) > 0

	if hasArgs {
		return nil, fmt.Errorf("unexpected args")
	}

	params := m.op.Parameters

	vals := make([]interface{}, 0, len(params))
	pairs := make([]interface{}, 0, len(params)<<1)
	for i, param := range params {
		kw := param.Name
		if !param.Required {
			kw += "?"
		}
		switch typ := param.SimpleSchema.Type; typ {
		case "array":
			vals = append(vals, (*starlark.List)(nil))
		case "string":
			vals = append(vals, "")
		default:
			if param.Schema == nil {
				return nil, fmt.Errorf("unknown type: %s", typ)
			}
			// ???
			vals = append(vals, (*starlark.Value)(nil))
		}
		pairs = append(pairs, kw, &vals[i])
	}

	fmt.Println("pairs", pairs)
	if err := starlark.UnpackArgs(m.name, args, kwargs, pairs...); err != nil {
		return nil, err
	}

	var (
		urlPath      = m.path
		urlVals      = url.Values{}
		body         io.Reader
		consumesType = "application/json" // TODO
		producesType = "application/json" // TODO
	)
	for i, param := range params {
		arg := vals[i]
		if arg == nil {
			continue // optional?
		}

		switch v := param.In; v {
		case "body":
			// create JSON?
			fmt.Println("body", vals[i])
			switch typ := consumesType; typ {
			case "application/json":
				v, ok := arg.(starlark.Value)
				if !ok {
					return nil, fmt.Errorf("unknown body arg: %T %v", arg, arg)
				}
				rsp, err := starlarkJSONEncode.CallInternal(
					thread, starlark.Tuple{v}, nil,
				)
				if err != nil {
					return nil, err
				}
				body = strings.NewReader(
					string(rsp.(starlark.String)),
				)

			default:
				return nil, fmt.Errorf("unknown consume type: %s", typ)
			}

		case "path":
			fmt.Println("path", vals[i])

		case "query":
			fmt.Println("query", vals[i])
			switch v := arg.(type) {
			case string:
				urlVals.Set(param.Name, v)
			case int:
				urlVals.Set(param.Name, strconv.Itoa(v))
			case *starlark.List:
				for i := 0; i < v.Len(); i++ {
					switch v := v.Index(i).(type) {
					case starlark.String:
						urlVals.Set(param.Name, string(v))
					case starlark.Int:
						x, _ := v.Int64()
						urlVals.Set(param.Name, strconv.Itoa(int(x)))
					default:
						return nil, fmt.Errorf("invalid param list type: %T %v", v, v)
					}
				}
			default:
				return nil, fmt.Errorf("invalid param type: %T %v", v, v)
			}
		default:
			return nil, fmt.Errorf("unhandled parameter in: %s", v)
		}
	}
	u := m.c.makeURL(urlPath, urlVals)

	urlStr := u.String()
	fmt.Println("urlStr", urlStr)

	req, err := http.NewRequestWithContext(ctx, m.method, urlStr, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", consumesType)
	req.Header.Set("Accepts", producesType)

	c := m.c.getClient()

	rsp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer rsp.Body.Close()

	rspTyp, ok := m.op.Responses.StatusCodeResponses[rsp.StatusCode]
	if !ok {
		return nil, fmt.Errorf("%s", rsp.Status)
	}
	fmt.Println("rspTyp", rspTyp)

	if rsp.StatusCode/100 != 2 {
		// TODO: error encoding.
		return nil, fmt.Errorf("%s", rsp.Status)
	}

	// Produce struct or array
	switch typ := producesType; typ {
	case "application/json":
		rspBody, err := ioutil.ReadAll(rsp.Body)
		if err != nil {
			return nil, err
		}

		bodyStr := starlark.String(rspBody)

		// Load schema
		val, err := starlarkJSONDecode.CallInternal(
			thread, starlark.Tuple{bodyStr}, nil,
		)
		if err != nil {
			return nil, err
		}

		return toStruct(rspTyp.Schema, val)
		//return val, err

	default:
		return nil, fmt.Errorf("unknown produces type: %s", typ)
	}
}

func errKeyValue(schema *spec.Schema, want string, v starlark.Value) error {
	return fmt.Errorf("invalid type for %s, want %s got %s", schema.ID, want, v.Type())
}

func typeStr(schema *spec.Schema) string {
	return strings.Join([]string(schema.Type), ",")
}

// TODO: build typed Dict and typed Lists.
func toStruct(schema *spec.Schema, v starlark.Value) (starlark.Value, error) {

	switch v := v.(type) {
	case *starlark.Dict:
		if typ := typeStr(schema); typ != "object" {
			return nil, errKeyValue(schema, "dict", v)
		}
		constructor := starlark.String(schema.ID)
		kwargs := v.Items()

		// TODO: validate spec.
		for _, kwarg := range kwargs {
			k, ok := starlark.AsString(kwarg[0])
			if !ok {
				return nil, fmt.Errorf("invalid key %s", k)
			}
			v := kwarg[1]

			keySchema, ok := schema.Properties[k]
			if !ok {
				return nil, fmt.Errorf("unpexpected key %s", k)
			}

			x, err := toStruct(&keySchema, v)
			if err != nil {
				return nil, err
			}
			kwarg[1] = x
		}

		s := starlarkstruct.FromKeywords(constructor, kwargs)
		return s, nil

	case *starlark.List:
		if typeStr(schema) != "array" {
			return nil, errKeyValue(schema, "list", v)
		}
		if items := schema.Items; items == nil || items.Schema == nil {
			return nil, fmt.Errorf("unepected items schema: %v", items)
		}
		keySchema := schema.Items.Schema

		// TODO: validate spec.
		for i := 0; i < v.Len(); i++ {
			x, err := toStruct(keySchema, v.Index(i))
			if err != nil {
				return nil, err
			}
			v.SetIndex(i, x)
		}
		return v, nil

	case starlark.String:
		switch typeStr(schema) {
		case "string", "password":
			return v, nil
		case "byte", "binary":
			data, err := base64.StdEncoding.DecodeString(string(v))
			if err != nil {
				return nil, err
			}
			return starlark.Bytes(string(data)), nil
		case "date":
			t, err := time.Parse("2006-Jan-02", string(v))
			if err != nil {
				return nil, err
			}
			return starlarktime.Time(t), nil
		case "date-time":
			t, err := time.Parse(time.RFC3339, string(v))
			if err != nil {
				return nil, err
			}
			return starlarktime.Time(t), nil
		default:
			return v, nil // TODO: warn?
		}

	case starlark.Int:
		if typeStr(schema) != "integer" {
			return nil, errKeyValue(schema, "int", v)
		}
		return v, nil

	case starlark.Float:
		if typeStr(schema) != "number" {
			return nil, errKeyValue(schema, "float", v)
		}
		return v, nil

	case starlark.Bool:
		if typeStr(schema) != "boolean" {
			return nil, errKeyValue(schema, "bool", v)
		}
		return v, nil

	default:
		// TODO: validate spec?
		return v, nil
	}
}

func NewMessage(schema *spec.Schema, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	hasArgs := len(args) > 0
	hasKwargs := len(kwargs) > 0

	if hasArgs && len(args) > 1 {
		return nil, fmt.Errorf("unexpected number of args")
	}

	if hasArgs && hasKwargs {
		return nil, fmt.Errorf("unxpected args and kwargs")
	}

	if hasArgs {
		return toStruct(schema, args[0])
	}

	return nil, fmt.Errorf("TODO: kwargs")
}