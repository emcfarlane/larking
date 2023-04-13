// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlarkopenapi

// OpenAPI spec:
// https://github.com/OAI/OpenAPI-Specification/blob/main/versions/2.0.md#dataTypeType

import (
	"bytes"
	"context"
	"fmt"
	"hash/crc32"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	starlarkjson "go.starlark.net/lib/json"
	"go.starlark.net/starlark"
	"gocloud.dev/blob"
	"larking.io/starlib/net/starlarkhttp"
	"larking.io/starlib/starext"
	"larking.io/starlib/starlarkrule"
	"larking.io/starlib/starlarkstruct"
	"larking.io/starlib/starlarkthread"

	openapiv2 "github.com/google/gnostic/openapiv2"
	openapiv3 "github.com/google/gnostic/openapiv3"
	surface "github.com/google/gnostic/surface"
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
	name   string
	addr   url.URL
	client *starlarkhttp.Client

	val   []byte
	docv2 *openapiv2.Document
	docv3 *openapiv3.Document
	model *surface.Model

	operationsv2 map[string]*openapiv2.Operation
	operationsv3 map[string]*openapiv3.Operation
}

var defaultClient = starlarkhttp.NewClient(http.DefaultClient)

func Open(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		addrStr string
		specStr string
		client  = defaultClient
	)
	if err := starlark.UnpackArgs(fnname, args, kwargs, "addr", &addrStr, "spec?", &specStr, "client?", &client); err != nil {
		return nil, err
	}

	addr, err := url.ParseRequestURI(addrStr)
	if err != nil {
		return nil, err
	}
	if specStr == "" {
		specStr = addrStr
	}

	ctx := starlarkthread.GetContext(thread)

	var val []byte
	if u, err := url.ParseRequestURI(specStr); err == nil {
		rsp, err := http.Get(u.String())
		if err != nil {
			return nil, err
		}
		defer rsp.Body.Close()

		b, err := io.ReadAll(rsp.Body)
		if err != nil {
			return nil, err
		}
		val = b
	} else {
		b, err := os.ReadFile(specStr)
		if err != nil {
			return nil, err
		}
		val = b
	}

	c := &Client{
		name:   specStr,
		addr:   *addr,
		val:    val,
		client: client,
	}
	if err := c.load(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Client) do(
	thread *starlark.Thread,
	fnname string,
	req *starlarkhttp.Request,
) (*starlarkhttp.Response, error) {
	return c.client.Do(thread, fnname, req)
}

func (c *Client) load(ctx context.Context) error {
	b := c.val

	if docv2, err := openapiv2.ParseDocument(b); err == nil {
		c.docv2 = docv2

		c.addr.Path = path.Join(c.addr.Path, docv2.BasePath)

		c.operationsv2 = make(map[string]*openapiv2.Operation)
		for _, item := range docv2.Paths.Path {
			for _, op := range [...]*openapiv2.Operation{
				item.Value.Get,
				item.Value.Put,
				item.Value.Post,
				item.Value.Delete,
				item.Value.Options,
				item.Value.Head,
				item.Value.Patch,
			} {
				if op == nil {
					continue
				}
				c.operationsv2[op.OperationId] = op
			}
		}

		m, err := surface.NewModelFromOpenAPI2(docv2, c.name)
		if err != nil {
			return err
		}
		//b, _ := json.MarshalIndent(m, "", "  ")
		//fmt.Println(string(b))
		c.model = m

	} else if docv3, err := openapiv3.ParseDocument(b); err == nil {
		c.docv3 = docv3

		c.operationsv3 = make(map[string]*openapiv3.Operation)
		for _, item := range docv3.Paths.Path {
			for _, op := range [...]*openapiv3.Operation{
				item.Value.Get,
				item.Value.Put,
				item.Value.Post,
				item.Value.Delete,
				item.Value.Options,
				item.Value.Head,
				item.Value.Patch,
			} {
				if op == nil {
					continue
				}
				c.operationsv3[op.OperationId] = op
			}
		}

		m, err := surface.NewModelFromOpenAPI3(docv3, c.name)
		if err != nil {
			return err
		}
		//b, _ := json.MarshalIndent(m, "", "  ")
		//fmt.Println(string(b))
		c.model = m

	} else {
		return err
	}
	return nil
}

func (c *Client) makeURL(urlPath string, urlQuery url.Values) url.URL {
	addr := c.addr
	addr.Path = path.Join(c.addr.Path, urlPath)
	addr.RawQuery = urlQuery.Encode()
	return addr
}

func (c *Client) String() string        { return fmt.Sprintf("<client %q>", c.name) }
func (c *Client) Type() string          { return "openapi.client" }
func (c *Client) Freeze()               {} // immutable?
func (c *Client) Truth() starlark.Bool  { return true }
func (c *Client) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: %s", c.Type()) }

func (c *Client) Attr(name string) (starlark.Value, error) {
	var method *surface.Method
	for _, m := range c.model.Methods {
		if m.Name == name {
			method = m
			break
		}
	}
	if method == nil {
		return nil, nil
	}
	m := &Method{c: c, m: method}
	if c.operationsv2 != nil {
		m.op2 = c.operationsv2[m.m.Operation]
	} else if c.operationsv3 != nil {
		m.op3 = c.operationsv3[m.m.Operation]
	}
	return m, nil
}
func (c *Client) AttrNames() []string {
	names := make([]string, 0, len(c.model.Methods))
	for _, m := range c.model.Methods {
		names = append(names, m.Name)
	}
	sort.Strings(names)
	return names
}

// "openapi.v2.Document"
// "openapi.v3.Document"
type Method struct {
	c   *Client
	m   *surface.Method
	op2 *openapiv2.Operation
	op3 *openapiv3.Operation
}

func (m *Method) String() string        { return fmt.Sprintf("<method %q>", m.m.Name) }
func (m *Method) Type() string          { return "openapi.method" }
func (m *Method) Freeze()               {} // immutable?
func (m *Method) Truth() starlark.Bool  { return m.m != nil }
func (m *Method) Hash() (uint32, error) { return starlark.String(m.m.Name).Hash() }

func (m *Method) getType(name string) (*surface.Type, error) {
	for _, typ := range m.c.model.Types {
		if typ.Name == name {
			return typ, nil
		}
	}
	return nil, fmt.Errorf("unknown type: %s", name)
}

func chooseType(typs []string) string {
	typ := "application/json"
	if n := len(typs); n > 0 {
		typ = typs[0]
	} else if n > 1 {
		typ = typs[0]
		for _, altTyp := range typs[1:] {
			if altTyp == "application/json" {
				typ = "application/json"
			}
		}
	}
	return typ
}

func (m *Method) Name() string { return m.m.Name }
func (m *Method) CallInternal(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	hasArgs := len(args) > 0
	hasKwargs := len(kwargs) > 0
	ctx := starlarkthread.GetContext(thread)

	if hasArgs && len(args) > 1 {
		return nil, fmt.Errorf("unexpected number of args")
	}

	if hasArgs && hasKwargs {
		return nil, fmt.Errorf("unxpected args and kwargs")
	}

	paramsType, err := m.getType(m.m.ParametersTypeName)
	if err != nil {
		return nil, err
	}

	fields := make(map[string]*surface.Field, len(paramsType.Fields))
	for _, fd := range paramsType.Fields {
		fields[fd.Name] = fd
	}

	var (
		urlPath      = m.m.Path
		urlValues    = make(url.Values)
		headers      = make(http.Header)
		body         io.Reader
		formWriter   *multipart.Writer
		consumesType = "application/json"
		producesType = "application/json"
	)
	if m.op2 != nil {
		consumesType = chooseType(m.op2.Consumes)
		producesType = chooseType(m.op2.Produces)
	}
	//else if m.op3 != nil {
	// TODO: handle consumes and produces.
	//}

	if hasArgs {
		switch v := args[0].(type) {
		//case starlark.IterableMapping:
		//case starlark.HasAttrs:
		default:
			return nil, fmt.Errorf("openapi: unsupported args conversion %s<%T>", v, v)
		}
	}

	for _, kwarg := range kwargs {
		k := string(kwarg[0].(starlark.String))
		v := kwarg[1]

		fd, ok := fields[k]
		if !ok {
			// Handle op3 special cases.
			// body -> request_body and then look for the consumesType field.
			if m.op3 != nil && k == "body" {
				fd, ok = fields["request_body"]
				if !ok {
					return nil, fmt.Errorf("unknown field: %q", k)
				}
				typ, err := m.getType(fd.Type)
				if err != nil {
					return nil, err
				}
				ok = false
				for _, f := range typ.Fields {
					if f.Name == consumesType {
						ok = true
						fd = f
						break
					}
				}
			}

			if !ok {
				return nil, fmt.Errorf("unknown field: %q", k)
			}
		}

		v, err := m.asField(fd, v)
		if err != nil {
			return nil, fmt.Errorf("invalid value for field %q: %w", k, err)
		}

		switch fd.Position {
		case surface.Position_BODY:
			rsp, err := starlark.Call(
				thread, starlarkJSONEncode, starlark.Tuple{v}, nil,
			)
			if err != nil {
				return nil, err
			}
			body = strings.NewReader(
				string(rsp.(starlark.String)),
			)

		case surface.Position_HEADER:
			switch v := v.(type) {
			case starlark.Iterable:
				iter := v.Iterate()
				defer iter.Done()

				var p starlark.Value
				for iter.Next(&p) {
					headers.Add(fd.Name, fmt.Sprintf("%#v", p))
				}

			default:
				headers.Add(fd.Name, fmt.Sprintf("%#v", v))
			}

		case surface.Position_FORMDATA:

			switch consumesType {
			case "multipart/form-data":
				if body == nil {
					buf := new(bytes.Buffer)
					formWriter = multipart.NewWriter(buf)
					// TODO: check this is okay.
					x := crc32.ChecksumIEEE([]byte(m.m.Path))
					if err := formWriter.SetBoundary(
						fmt.Sprintf("%x%x%x", x, x, x),
					); err != nil {
						return nil, err
					}
					body = buf
				}

				// TODO: handle file encoding
				s := fmt.Sprintf("%#v", v)
				filename := filepath.Base(s)

				label, err := starlarkrule.ParseRelativeLabel(thread.Name, s)
				if err != nil {
					return nil, err
				}
				bktURL := label.BucketURL()
				key := label.Key()
				bkt, err := blob.OpenBucket(ctx, bktURL)
				if err != nil {
					return nil, err
				}
				defer bkt.Close()

				r, err := bkt.NewReader(ctx, key, nil)
				if err != nil {
					return nil, err
				}
				defer r.Close()

				formFile, err := formWriter.CreateFormFile(fd.Name, filename)
				if err != nil {
					return nil, err
				}
				if _, err := io.Copy(formFile, r); err != nil {
					return nil, err
				}

				//if err := formWriter.WriteField(fd.Name, s); err != nil {
				//	return nil, err
				//}

			case "application/x-www-form-urlencoded":
				return nil, fmt.Errorf("unimplemented consume type: %s", consumesType)

			default:
				return nil, fmt.Errorf("unexpected consumes type %v for \"formData\"", consumesType)
			}

		case surface.Position_QUERY:
			switch v := v.(type) {
			case starlark.Iterable:
				iter := v.Iterate()
				defer iter.Done()

				var p starlark.Value
				for iter.Next(&p) {
					urlValues.Add(fd.Name, fmt.Sprintf("%#v", p))
				}

			default:
				urlValues.Set(fd.Name, fmt.Sprintf("%#v", v))
			}

		case surface.Position_PATH:
			key := "{" + fd.Name + "}"
			val := fmt.Sprintf("%#v", v)
			urlPath = strings.ReplaceAll(urlPath, key, val)
		}
	}

	if formWriter != nil {
		if err := formWriter.Close(); err != nil {
			return nil, err
		}
		consumesType = formWriter.FormDataContentType()
	}

	headers.Set("Accept", producesType)
	if body != nil {
		headers.Set("Content-Type", consumesType)
	}

	u := m.c.makeURL(urlPath, urlValues)
	urlStr := u.String()

	req, err := http.NewRequestWithContext(ctx, m.m.Method, urlStr, body)
	if err != nil {
		return nil, err
	}
	req.Header = headers

	rsp, err := m.c.do(thread, m.m.Name, &starlarkhttp.Request{
		Request: req,
	})
	if err != nil {
		return nil, err
	}
	defer rsp.Body.Close()

	//rspTyp, rspOk := m.op.Responses.StatusCodeResponses[rsp.StatusCode]
	//rspDef := m.op.Responses.Default

	// Produce struct or array
	switch producesType {
	case "application/json":
		if m.m.ResponsesTypeName == "" {
			return starlark.None, nil
		}

		rspBody, err := io.ReadAll(rsp.Body)
		if err != nil {
			return nil, err
		}
		bodyStr := starlark.String(rspBody)

		// Load schema
		val, err := starlark.Call(
			thread, starlarkJSONDecode, starlark.Tuple{bodyStr}, nil,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}

		responseType, err := m.getType(m.m.ResponsesTypeName)
		if err != nil {
			return nil, fmt.Errorf("failed to get response type: %w", err)
		}

		rspKey := strconv.Itoa(rsp.StatusCode)
		var rspField *surface.Field
		for _, fd := range responseType.Fields {
			if fd.Name == rspKey {
				rspField = fd

				// TODO: op3 encode content/type in subfields,
				// find the right one.
				// TODO: https://github.com/google/gnostic/pull/385
				if m.op3 != nil {
					op3Type, err := m.getType(rspField.Type)
					if err != nil {
						return nil, fmt.Errorf("failed to get response type: %w", err)
					}
					for _, fd := range op3Type.Fields {
						if fd.Name == producesType {
							rspField = fd
							break
						}
					}
				}
				break
			}
		}

		if rspField == nil {
			return val, fmt.Errorf("unknown response type %s: %q", rsp.Status, val)
		}
		if rsp.StatusCode/100 != 2 {
			return val, fmt.Errorf("%s: %q", rsp.Status, val)
		}
		return m.asField(rspField, val)

	default:
		return nil, fmt.Errorf("%s: unknown produces type: %s", rsp.Status, producesType)
	}
}

func typeError(typ *surface.Type, v starlark.Value) error {
	return fmt.Errorf("invalid type for %s, want %s got %s", typ.GetName(), typ.GetKind(), v.Type())
}

var (
	starlarkJSONEncode = starlarkjson.Module.Members["encode"].(*starlark.Builtin)
	starlarkJSONDecode = starlarkjson.Module.Members["decode"].(*starlark.Builtin)
)

func (m *Method) toField(name string) *surface.Field {
	if _, err := m.getType(name); err == nil {
		return &surface.Field{Type: name, Kind: surface.FieldKind_REFERENCE}
	}

	switch name {
	case "string", "integer", "number", "boolean":
		return &surface.Field{Type: name, Kind: surface.FieldKind_SCALAR}
	case "int32", "int64":
		return &surface.Field{Type: "integer", Format: name, Kind: surface.FieldKind_SCALAR}
	case "float", "double":
		return &surface.Field{Type: "number", Format: name, Kind: surface.FieldKind_SCALAR}
	case "date":
		// date – full-date notation as defined by RFC 3339, section 5.6, for example, 2017-07-21
		return &surface.Field{Type: "string", Format: name, Kind: surface.FieldKind_SCALAR}
	case "date-time":
		// date-time – the date-time notation as defined by RFC 3339, section 5.6, for example, 2017-07-21T17:32:28Z
		return &surface.Field{Type: "string", Format: name, Kind: surface.FieldKind_SCALAR}
	case "password", "byte", "binrary":
		return &surface.Field{Type: "string", Format: name, Kind: surface.FieldKind_SCALAR}
	}
	// ???
	return &surface.Field{Type: "string", Format: name, Kind: surface.FieldKind_SCALAR}
}

func (m *Method) asField(fd *surface.Field, v starlark.Value) (starlark.Value, error) {
	switch fd.Kind {
	case surface.FieldKind_SCALAR:
		switch fd.Type {
		case "string":
			if x, ok := v.(starlark.String); ok {
				return x, nil
			}
			if v == starlark.None {
				return starlark.String(""), nil
			}
		case "integer":
			if v == starlark.None {
				return starlark.MakeInt(0), nil
			}
			return starlark.NumberToInt(v)
		case "number":
			switch x := v.(type) {
			case starlark.Float:
				return x, nil
			case starlark.Int:
				return x.Float(), nil
			case starlark.NoneType:
				return starlark.Float(0), nil
			}
		case "boolean":
			if b, ok := v.(starlark.Bool); ok {
				return b, nil
			}
			if v == starlark.None {
				return starlark.Bool(false), nil
			}
		}
	case surface.FieldKind_MAP:
		switch v := v.(type) {
		case *starlark.Dict:
			// always "map[string]<something>"
			name := strings.TrimPrefix(fd.Type, "map[string]")
			subFd := m.toField(name)

			for _, key := range v.Keys() {
				val, _, err := v.Get(key)
				if err != nil {
					return nil, err
				}

				val, err = m.asField(subFd, val)
				if err != nil {
					return nil, err
				}

				v.SetKey(key, val)
			}
			return v, nil

		case starlark.NoneType:
			return starlark.NewDict(0), nil
		}
	case surface.FieldKind_ARRAY:
		switch v := v.(type) {
		case starlark.HasSetIndex:
			subFd := m.toField(fd.Type)

			n := v.Len()
			for i := 0; i < n; i++ {
				val, err := m.asField(subFd, v.Index(i))
				if err != nil {
					return nil, err
				}
				v.SetIndex(i, val)
			}
			return v, nil
		case starlark.NoneType:
			return starlark.NewList(nil), nil
		}
	case surface.FieldKind_REFERENCE:
		typ, err := m.getType(fd.Type)
		if err != nil {
			return nil, err
		}
		return m.asObject(typ, v)
	case surface.FieldKind_ANY:
		return v, nil
	default:
		panic(fmt.Sprintf("unknown kind %q", fd.Kind.String()))
	}
	return starlark.None, fmt.Errorf(
		"openapi: invalid type conversion %s<%T> to %s", v, v, fd.Kind.String(),
	)
}

func (m *Method) asObject(typ *surface.Type, arg starlark.Value) (starlark.Value, error) {
	switch typ.Kind {
	case surface.TypeKind_STRUCT:
		switch arg := arg.(type) {
		case *starlark.Dict:
			index := make(map[string]starlark.Value)
			kwargs := arg.Items()
			for _, kwarg := range kwargs {
				key, val := kwarg[0], kwarg[1]

				k, ok := starlark.AsString(key)
				if !ok {
					return nil, fmt.Errorf("invalid key %s", k)
				}
				index[k] = val
			}

			items := make([]starlark.Tuple, 0, len(typ.Fields))
			array := make([]starlark.Value, len(typ.Fields)*2) // allocate a single backing array
			for _, fd := range typ.Fields {
				val, ok := index[fd.Name]
				if !ok {
					val = starlark.None
				}

				x, err := m.asField(fd, val)
				if err != nil {
					return nil, err
				}

				pair := starlark.Tuple(array[:2:2])
				array = array[2:]
				pair[0] = starlark.String(fd.Name)
				pair[1] = x
				items = append(items, pair)
			}

			constructor := starlarkstruct.Default
			s := starlarkstruct.FromKeywords(constructor, items)
			return s, nil

		case *starlarkstruct.Struct:
			index := make(map[string]starlark.Value)
			n := arg.Len()
			for i := 0; i < n; i++ {
				k, v := arg.KeyIndex(i)
				index[k] = v
			}
			for _, fd := range typ.Fields {
				val, ok := index[fd.Name]
				if !ok {
					continue // skip for now, partial structs
				}

				x, err := m.asField(fd, val)
				if err != nil {
					return nil, err
				}
				arg.SetField(fd.Name, x)
			}
			return arg, nil

		case starlark.NoneType:
			return starlark.None, nil // objects can be nil

		default:
			return nil, typeError(typ, arg)
		}

	case surface.TypeKind_OBJECT:
		switch arg := arg.(type) {
		case *starlark.Dict:

			index := make(map[string]starlark.Value)
			kwargs := arg.Items()
			for _, kwarg := range kwargs {
				key, val := kwarg[0], kwarg[1]

				k, ok := starlark.AsString(key)
				if !ok {
					return nil, fmt.Errorf("invalid key %s", k)
				}
				index[k] = val
			}

			for _, fd := range typ.Fields {
				val, ok := index[fd.Name]
				if !ok {
					val = starlark.None
				}

				x, err := m.asField(fd, val)
				if err != nil {
					return nil, err
				}
				arg.SetKey(starlark.String(fd.Name), x)

			}
			return arg, nil

		default:
			return nil, typeError(typ, arg)
		}
	default:
		panic(fmt.Sprintf("unknown kind: %v", typ.Kind))
	}
}
