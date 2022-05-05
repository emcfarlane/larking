// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package blob provides access to blob objects within a storage location.
package starlarkblob

import (
	"fmt"
	"sort"

	"github.com/emcfarlane/larking/starlib/starext"
	"github.com/emcfarlane/larking/starlib/starlarkstruct"
	"github.com/emcfarlane/larking/starlib/starlarkthread"

	starlarktime "go.starlark.net/lib/time"
	"go.starlark.net/starlark"
	"gocloud.dev/blob"
)

func NewModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "blob",
		Members: starlark.StringDict{
			"open": starext.MakeBuiltin("blob.open", Open),
		},
	}
}

func Open(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 1, &name); err != nil {
		return nil, err
	}

	ctx := starlarkthread.Context(thread)
	bkt, err := blob.OpenBucket(ctx, name)
	if err != nil {
		return nil, err
	}

	b := &Bucket{name: name, bkt: bkt}
	if err := starlarkthread.AddResource(thread, b); err != nil {
		return nil, err
	}
	return b, nil
}

type Bucket struct {
	name string
	bkt  *blob.Bucket
}

func (b *Bucket) Close() error          { return b.bkt.Close() }
func (b *Bucket) String() string        { return fmt.Sprintf("<bucket %q>", b.name) }
func (b *Bucket) Type() string          { return "blob.bucket" }
func (b *Bucket) Freeze()               {} // concurrent safe
func (b *Bucket) Truth() starlark.Bool  { return b.bkt != nil }
func (b *Bucket) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: %s", b.Type()) }

type bucketAttr func(b *Bucket) starlark.Value

var bucketAttrs = map[string]bucketAttr{
	"attributes": func(b *Bucket) starlark.Value { return starext.MakeMethod(b, "attributes", b.attributes) },
	"write_all":  func(b *Bucket) starlark.Value { return starext.MakeMethod(b, "write_all", b.writeAll) },
	"read_all":   func(b *Bucket) starlark.Value { return starext.MakeMethod(b, "read_all", b.readAll) },
	"delete":     func(b *Bucket) starlark.Value { return starext.MakeMethod(b, "delete", b.delete) },
	"close":      func(b *Bucket) starlark.Value { return starext.MakeMethod(b, "close", b.close) },
}

func (v *Bucket) Attr(name string) (starlark.Value, error) {
	if a := bucketAttrs[name]; a != nil {
		return a(v), nil
	}
	return nil, nil
}
func (v *Bucket) AttrNames() []string {
	names := make([]string, 0, len(bucketAttrs))
	for name := range bucketAttrs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (v *Bucket) attributes(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		key string
	)
	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 1, &key); err != nil {
		return nil, err
	}

	ctx := starlarkthread.Context(thread)
	p, err := v.bkt.Attributes(ctx, key)
	if err != nil {
		return nil, err
	}
	return starlarkstruct.FromKeyValues(
		starlarkstruct.Default,
		"cache_control", starlark.String(p.CacheControl),
		"content_disposition", starlark.String(p.ContentDisposition),
		"content_encoding", starlark.String(p.ContentEncoding),
		"content_language", starlark.String(p.ContentLanguage),
		"content_type", starlark.String(p.ContentType),
		"metadata", starext.ToDict(p.Metadata),
		"mod_time", starlarktime.Time(p.ModTime),
		"sie", starlark.MakeInt64(p.Size),
		"md5", starlark.String(p.MD5),
		"etag", starlark.String(p.ETag),
	), nil

}

func (v *Bucket) writeAll(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		key   string
		bytes string
		opts  blob.WriterOptions // TODO

		//
		contentMD5  string // []byte
		metadata    starlark.Dict
		beforeWrite starlark.Callable
	)

	if err := starlark.UnpackArgs(fnname, args, kwargs,
		"key", &key,
		"bytes", &bytes,
		"buffer_size?", &opts.BufferSize, // int
		"cache_control?", &opts.CacheControl, //  string
		"content_disposition?", &opts.ContentDisposition, //  string
		"content_encoding?", &opts.ContentEncoding, // string
		"content_language?", &opts.ContentLanguage, // string
		"content_type?", &opts.ContentType, // string
		"content_md5?", &contentMD5, // []byte
		"metadata?", &metadata, // map[string]string
		"before_write?", &beforeWrite, // func(asFunc func(interface{}) bool) error

	); err != nil {
		return nil, err
	}
	opts.ContentMD5 = []byte(contentMD5)
	if items := metadata.Items(); len(items) > 0 {
		opts.Metadata = make(map[string]string, len(items))

		for _, item := range items {
			key, ok := starlark.AsString(item[0])
			if !ok {
				return nil, fmt.Errorf("invalid metadata key: %v", item[0])
			}
			val, ok := starlark.AsString(item[1])
			if !ok {
				return nil, fmt.Errorf("invalid metadata key: %v, value: %v", item[0], item[1])
			}
			opts.Metadata[key] = val
		}
	}
	// TODO: handle As method beforeWrite.

	ctx := starlarkthread.Context(thread)
	if err := v.bkt.WriteAll(ctx, key, []byte(bytes), &opts); err != nil {
		return nil, err
	}
	return starlark.None, nil
}

func (v *Bucket) readAll(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		key string
		//opts blob.ReaderOptions
	)
	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 1, &key); err != nil {
		return nil, err
	}

	ctx := starlarkthread.Context(thread)
	p, err := v.bkt.ReadAll(ctx, key) // &opts)
	if err != nil {
		return nil, err
	}
	return starlark.Bytes(p), nil
}

func (v *Bucket) delete(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		key string
	)
	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 1, &key); err != nil {
		return nil, err
	}

	ctx := starlarkthread.Context(thread)
	if err := v.bkt.Delete(ctx, key); err != nil {
		return nil, err
	}
	return starlark.None, nil
}

func (v *Bucket) close(_ *starlark.Thread, fnname string, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if err := v.bkt.Close(); err != nil {
		return nil, err
	}
	return starlark.None, nil
}
