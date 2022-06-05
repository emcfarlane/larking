// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package http provides HTTP client implementations.
package starlarkhttp

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/emcfarlane/larking/starlib/starext"
	"github.com/emcfarlane/larking/starlib/starlarkerrors"
	"github.com/emcfarlane/larking/starlib/starlarkio"
	"github.com/emcfarlane/larking/starlib/starlarkstruct"
	"github.com/emcfarlane/larking/starlib/starlarkthread"
	"go.starlark.net/starlark"
)

// NewModule loads the predeclared built-ins for the net/http module.
func NewModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "http",
		Members: starlark.StringDict{
			"default_client": defaultClient,
			"get":            starext.MakeBuiltin("http.get", defaultClient.get),
			"head":           starext.MakeBuiltin("http.head", defaultClient.head),
			"post":           starext.MakeBuiltin("http.post", defaultClient.post),
			"client":         starext.MakeBuiltin("http.new_client", MakeClient),
			"request":        starext.MakeBuiltin("http.new_request", MakeRequest),

			// net/http errors
			"err_not_supported":         starlarkerrors.NewError(http.ErrNotSupported),
			"err_missing_boundary":      starlarkerrors.NewError(http.ErrMissingBoundary),
			"err_not_multipart":         starlarkerrors.NewError(http.ErrNotMultipart),
			"err_body_not_allowed":      starlarkerrors.NewError(http.ErrBodyNotAllowed),
			"err_hijacked":              starlarkerrors.NewError(http.ErrHijacked),
			"err_content_length":        starlarkerrors.NewError(http.ErrContentLength),
			"err_abort_handler":         starlarkerrors.NewError(http.ErrAbortHandler),
			"err_body_read_after_close": starlarkerrors.NewError(http.ErrBodyReadAfterClose),
			"err_handler_timeout":       starlarkerrors.NewError(http.ErrHandlerTimeout),
			"err_line_too_long":         starlarkerrors.NewError(http.ErrLineTooLong),
			"err_missing_file":          starlarkerrors.NewError(http.ErrMissingFile),
			"err_no_cookie":             starlarkerrors.NewError(http.ErrNoCookie),
			"err_no_location":           starlarkerrors.NewError(http.ErrNoLocation),
			"err_server_closed":         starlarkerrors.NewError(http.ErrServerClosed),
			"err_skip_alt_protocol":     starlarkerrors.NewError(http.ErrSkipAltProtocol),
			"err_use_last_response":     starlarkerrors.NewError(http.ErrUseLastResponse),
		},
	}
}

type Client struct {
	do     func(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error)
	frozen bool
}

func NewClient(client *http.Client) *Client {
	do := func(
		thread *starlark.Thread,
		fnname string,
		args starlark.Tuple,
		kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		var req *Request
		if err := starlark.UnpackArgs(fnname, args, kwargs,
			"req", &req,
		); err != nil {
			return nil, err
		}

		response, err := client.Do(req.Request)
		if err != nil {
			return nil, err
		}

		rsp := &Response{
			Response: response,
		}
		if err := starlarkthread.AddResource(thread, rsp); err != nil {
			return nil, err
		}
		return rsp, nil
	}
	return &Client{do: do}
}

func (v *Client) String() string        { return "<client>" }
func (v *Client) Type() string          { return "http.client" }
func (v *Client) Freeze()               { v.frozen = true }
func (v *Client) Truth() starlark.Bool  { return true }
func (v *Client) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: %s", v.Type()) }

type clientAttr func(*Client) starlark.Value

var clientAttrs = map[string]clientAttr{
	"do":   func(v *Client) starlark.Value { return starext.MakeMethod(v, "do", v.do) },
	"get":  func(v *Client) starlark.Value { return starext.MakeMethod(v, "get", v.get) },
	"post": func(v *Client) starlark.Value { return starext.MakeMethod(v, "post", v.post) },
	"head": func(v *Client) starlark.Value { return starext.MakeMethod(v, "head", v.head) },
}

func (v *Client) Attr(name string) (starlark.Value, error) {
	if a := clientAttrs[name]; a != nil {
		return a(v), nil
	}
	return nil, nil
}
func (v *Client) AttrNames() []string {
	names := make([]string, 0, len(clientAttrs))
	for name := range clientAttrs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

var defaultClient = NewClient(http.DefaultClient)

func (v *Client) Do(thread *starlark.Thread, fnname string, req *Request) (*Response, error) {
	x, err := v.do(thread, fnname, starlark.Tuple{req}, nil)
	if err != nil {
		return nil, err
	}
	w, ok := x.(*Response)
	if !ok {
		return nil, fmt.Errorf("invalid response type: %T", x)
	}
	return w, nil
}

func Do(thread *starlark.Thread, fnname string, req *Request) (*Response, error) {
	return defaultClient.Do(thread, fnname, req)
}

func (v *Client) get(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var urlstr string
	if err := starlark.UnpackArgs(fnname, args, kwargs,
		"url", &urlstr,
	); err != nil {
		return nil, err
	}

	ctx := starlarkthread.GetContext(thread)
	req, err := http.NewRequestWithContext(ctx, "GET", urlstr, nil)
	if err != nil {
		return nil, err
	}
	r := &Request{Request: req}
	return v.Do(thread, fnname, r)
}

func (v *Client) head(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var urlstr string
	if err := starlark.UnpackArgs(fnname, args, kwargs,
		"url", &urlstr,
	); err != nil {
		return nil, err
	}

	ctx := starlarkthread.GetContext(thread)
	req, err := http.NewRequestWithContext(ctx, "HEAD", urlstr, nil)
	if err != nil {
		return nil, err
	}
	r := &Request{Request: req}
	return v.Do(thread, fnname, r)
}

func (v *Client) post(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		urlstr      string
		contentType string
		body        starlark.Value
	)
	if err := starlark.UnpackArgs(fnname, args, kwargs,
		"url", &urlstr, "content_type", &contentType, "body", &body,
	); err != nil {
		return nil, err
	}
	rdr, err := makeReader(body)
	if err != nil {
		return nil, err
	}

	ctx := starlarkthread.GetContext(thread)
	req, err := http.NewRequestWithContext(ctx, "HEAD", urlstr, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	r := &Request{Request: req}
	return v.Do(thread, fnname, r)
}

func MakeClient(_ *starlark.Thread, name string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var fn starlark.Callable
	if err := starlark.UnpackPositionalArgs(name, args, kwargs, 1, &fn); err != nil {
		return nil, err
	}

	return &Client{
		do: func(
			thread *starlark.Thread,
			_ string,
			args starlark.Tuple,
			kwargs []starlark.Tuple,
		) (starlark.Value, error) {
			return starlark.Call(thread, fn, args, kwargs)
		},
	}, nil
}

type Request struct {
	*http.Request
	frozen bool
}

func makeReader(v starlark.Value) (io.Reader, error) {
	switch x := v.(type) {
	case starlark.String:
		return strings.NewReader(string(x)), nil
	case starlark.Bytes:
		return strings.NewReader(string(x)), nil
	case io.Reader:
		return x, nil
	case starlark.NoneType, nil:
		return http.NoBody, nil // none
	default:
		return nil, fmt.Errorf("unsupport type: %T", v)
	}
}

func MakeRequest(thread *starlark.Thread, name string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		method string
		urlstr string
		body   starlark.Value
	)
	if err := starlark.UnpackArgs(name, args, kwargs,
		"method", &method, "url", &urlstr, "body?", &body,
	); err != nil {
		return nil, err
	}

	r, err := makeReader(body)
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequest(method, urlstr, r)
	if err != nil {
		return nil, err
	}
	ctx := starlarkthread.GetContext(thread)
	request = request.WithContext(ctx)

	return &Request{
		Request: request,
	}, nil

}

func (v *Request) String() string {
	return fmt.Sprintf("<request %s %s>", v.Request.Method, v.Request.URL.String())
}
func (v *Request) Type() string          { return "nethttp.request" }
func (v *Request) Freeze()               { v.frozen = true }
func (v *Request) Truth() starlark.Bool  { return v.Request != nil }
func (v *Request) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: %s", v.Type()) }

type requestAttr struct {
	get func(*Request) (starlark.Value, error)
	set func(*Request, starlark.Value) error
}

var requestAttrs = map[string]requestAttr{
	"method": {
		get: func(v *Request) (starlark.Value, error) {
			return starlark.String(v.Method), nil
		},
		set: func(v *Request, val starlark.Value) error {
			var x int
			if err := starlark.AsInt(val, &x); err != nil {
				return err
			}
			v.ProtoMajor = x
			return nil
		},
	},
	"url": {
		get: func(v *Request) (starlark.Value, error) {
			return starlark.String(v.URL.String()), nil
		},
		set: func(v *Request, val starlark.Value) error {
			switch t := val.(type) {
			case starlark.String:
				u, err := url.Parse(string(t))
				if err != nil {
					return err
				}
				v.URL = u
				return nil
			default:
				return fmt.Errorf("expected string")
			}
		},
	},
	"proto": {
		get: func(v *Request) (starlark.Value, error) {
			return starlark.String(v.Proto), nil
		},
		set: func(v *Request, val starlark.Value) error {
			s, ok := starlark.AsString(val)
			if !ok {
				return fmt.Errorf("expected string")
			}
			v.Proto = s
			return nil
		},
	},
	"proto_major": {
		get: func(v *Request) (starlark.Value, error) {
			return starlark.MakeInt(v.ProtoMajor), nil
		},
		set: func(v *Request, val starlark.Value) error {
			var x int
			if err := starlark.AsInt(val, &x); err != nil {
				return err
			}
			v.ProtoMajor = x
			return nil
		},
	},
	"proto_minor": {
		get: func(v *Request) (starlark.Value, error) {
			return starlark.MakeInt(v.ProtoMinor), nil
		},
		set: func(v *Request, val starlark.Value) error {
			var x int
			if err := starlark.AsInt(val, &x); err != nil {
				return err
			}
			v.ProtoMinor = x
			return nil
		},
	},
	"header": {
		get: func(v *Request) (starlark.Value, error) {
			return &Header{
				header: v.Header,
				frozen: &v.frozen,
			}, nil
		},
		set: func(v *Request, val starlark.Value) error {
			switch t := val.(type) {
			case *Header:
				v.Header = t.header
				return nil
			default:
				return fmt.Errorf("expected http.header")
			}
		},
	},
	"body": {
		get: func(v *Request) (starlark.Value, error) {
			return &starlarkio.Reader{Reader: v.Body}, nil
		},
		set: func(v *Request, val starlark.Value) error {
			switch t := val.(type) {
			case io.ReadCloser:
				v.Body = t
				return nil
			default:
				return fmt.Errorf("expected io.read_closer")
			}
		},
	},
	"content_length": {
		get: func(v *Request) (starlark.Value, error) {
			return starlark.MakeInt64(v.ContentLength), nil
		},
		set: func(v *Request, val starlark.Value) error {
			var x int64
			if err := starlark.AsInt(val, &x); err != nil {
				return err
			}
			v.ContentLength = x
			return nil
		},
	},
	"basic_auth": {
		get: func(v *Request) (starlark.Value, error) {
			if username, password, ok := v.BasicAuth(); ok {
				return starlark.Tuple{
					starlark.String(username),
					starlark.String(password),
				}, nil
			}
			return starlark.None, nil
		},
		set: func(v *Request, val starlark.Value) error {
			tpl, ok := val.(starlark.Tuple)
			if !ok || len(tpl) != 2 {
				return fmt.Errorf("expected length 2 tuple")
			}
			username, ok := starlark.AsString(tpl[0])
			if !ok {
				return fmt.Errorf("invalid type for username")
			}
			password, ok := starlark.AsString(tpl[1])
			if !ok {
				return fmt.Errorf("invalid type for password")
			}
			v.SetBasicAuth(username, password)
			return nil
		},
	},
}

func (v *Request) Attr(name string) (starlark.Value, error) {
	if fn, ok := requestAttrs[name]; ok {
		return fn.get(v)
	}
	return nil, nil
}
func (v *Request) AttrNames() []string {
	names := make([]string, 0, len(requestAttrs))
	for name := range requestAttrs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
func (v *Request) SetField(name string, val starlark.Value) error {
	if fn, ok := requestAttrs[name]; ok {
		return fn.set(v, val)
	}
	return fmt.Errorf("unknown attribute %s", name)
}

type Response struct {
	*http.Response
	frozen bool
}

func (v *Response) Close() error          { return v.Body.Close() }
func (v *Response) String() string        { return fmt.Sprintf("<response %s>", v.Status) }
func (v *Response) Type() string          { return "nethttp.response" }
func (v *Response) Freeze()               { v.frozen = true }
func (v *Response) Truth() starlark.Bool  { return v.Response != nil }
func (v *Response) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: %s", v.Type()) }

// TODO: optional methods io.Closer, etc.
var responseAttrs = map[string]func(v *Response) (starlark.Value, error){
	"body": func(v *Response) (starlark.Value, error) {
		return &starlarkio.Reader{Reader: v.Body}, nil
	},
}

func (v *Response) Attr(name string) (starlark.Value, error) {
	if fn, ok := responseAttrs[name]; ok {
		return fn(v)
	}
	return nil, nil
}
func (v *Response) AttrNames() []string {
	names := make([]string, 0, len(responseAttrs))
	for name := range responseAttrs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

type Header struct {
	header http.Header
	frozen *bool
}

func (h *Header) String() string        { return fmt.Sprintf("%+v", h.header) }
func (h *Header) Type() string          { return "http.header" }
func (h *Header) Freeze()               { *h.frozen = true }
func (h *Header) Truth() starlark.Bool  { return h.header != nil }
func (h *Header) Hash() (uint32, error) { return 0, nil }

type headerAttr func(*Header) (starlark.Value, error)

var headerAttrs = map[string]headerAttr{
	"add": func(h *Header) (starlark.Value, error) {
		return starext.MakeMethod(h, "add", h.add), nil
	},
	"get": func(h *Header) (starlark.Value, error) {
		return starext.MakeMethod(h, "get", h.get), nil
	},
	"del": func(h *Header) (starlark.Value, error) {
		return starext.MakeMethod(h, "del", h.del), nil
	},
	"set": func(h *Header) (starlark.Value, error) {
		return starext.MakeMethod(h, "set", h.set), nil
	},
	"values": func(h *Header) (starlark.Value, error) {
		return starext.MakeMethod(h, "values", h.values), nil
	},
}

func (v *Header) Attr(name string) (starlark.Value, error) {
	if fn, ok := headerAttrs[name]; ok {
		return fn(v)
	}
	return nil, nil
}
func (v *Header) AttrNames() []string {
	names := make([]string, 0, len(headerAttrs))
	for name := range responseAttrs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
func (v *Header) add(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var key, value string
	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 2, &key, &value); err != nil {
		return nil, err
	}
	v.header.Add(key, value)
	return starlark.None, nil
}
func (v *Header) get(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var key string
	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 1, &key); err != nil {
		return nil, err
	}
	value := v.header.Get(key)
	return starlark.String(value), nil
}
func (v *Header) del(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var key string
	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 1, &key); err != nil {
		return nil, err
	}
	v.header.Del(key)
	return starlark.None, nil
}
func (v *Header) set(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var key, value string
	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 2, &key, &value); err != nil {
		return nil, err
	}
	v.header.Set(key, value)
	return starlark.None, nil
}
func (v *Header) values(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var key string
	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 1, &key); err != nil {
		return nil, err
	}
	values := v.header.Values(key)
	elems := make([]starlark.Value, len(values))
	for i, v := range values {
		elems[i] = starlark.String(v)
	}
	return starlark.NewList(elems), nil
}
