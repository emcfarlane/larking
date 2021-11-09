// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlarkblob

import (
	"fmt"
	"sort"

	"github.com/emcfarlane/larking/starlarkthread"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"gocloud.dev/blob"
)

func NewModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "blob",
		Members: starlark.StringDict{
			"open": starlark.NewBuiltin("blob.open", Open),
		},
	}
}

func Open(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackPositionalArgs("blob.open", args, kwargs, 1, &name); err != nil {
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

	frozen bool
}

func (b *Bucket) Close() error          { return b.bkt.Close() }
func (b *Bucket) String() string        { return fmt.Sprintf("<bucket %q>", b.name) }
func (b *Bucket) Type() string          { return "blob.bucket" }
func (b *Bucket) Freeze()               { b.frozen = true }
func (b *Bucket) Truth() starlark.Bool  { return b.bkt != nil }
func (b *Bucket) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: %s", b.Type()) }

var bucketMethods = map[string]*starlark.Builtin{
	"write_all": starlark.NewBuiltin("blob.bucket.write_all", bucketWriteAll),
	"read_all":  starlark.NewBuiltin("blob.bucket.read_all", bucketReadAll),
	"close":     starlark.NewBuiltin("blob.bucket.close", bucketClose),
}

func (v *Bucket) Attr(name string) (starlark.Value, error) {
	b := bucketMethods[name]
	if b == nil {
		return nil, nil
	}
	return b.BindReceiver(v), nil
}
func (v *Bucket) AttrNames() []string {
	names := make([]string, 0, len(bucketMethods))
	for name := range bucketMethods {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func bucketWriteAll(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("group.go: missing function arg")
	}
	v := b.Receiver().(*Bucket)
	if v.frozen {
		return nil, fmt.Errorf("bucket: frozen")
	}

	var (
		key   string
		bytes string
		opts  blob.WriterOptions // TODO

		//
		contentMD5  string // []byte
		metadata    starlark.Dict
		beforeWrite starlark.Callable
	)

	if err := starlark.UnpackArgs("blob.bucket.write_all", args, kwargs,
		"key", &key, "bytes", &bytes,
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

func bucketReadAll(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("group.go: missing function arg")
	}

	v := b.Receiver().(*Bucket)
	if v.frozen {
		return nil, fmt.Errorf("bucket: frozen")
	}

	var (
		key string
		//opts blob.ReaderOptions
	)
	if err := starlark.UnpackPositionalArgs("blob.bucket.read_all", args, kwargs, 1, &key); err != nil {
		return nil, err
	}

	ctx := starlarkthread.Context(thread)
	p, err := v.bkt.ReadAll(ctx, key) // &opts)
	if err != nil {
		return nil, err
	}
	return starlark.Bytes(p), nil
}

func bucketClose(_ *starlark.Thread, b *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	v := b.Receiver().(*Bucket)
	if err := v.bkt.Close(); err != nil {
		return nil, err
	}
	return starlark.None, nil
}
