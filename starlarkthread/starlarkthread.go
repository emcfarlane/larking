// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlarkthread

import (
	"context"
	"io"

	"go.starlark.net/starlark"
)

const ctxkey = "context"

// SetThreadContext sets the thread context.
func SetContext(thread *starlark.Thread, ctx context.Context) {
	thread.SetLocal(ctxkey, ctx)
}

// Context is a simple helper for getting a threads context.
// ctx := starlarkthread.Context(thread)
func Context(thread *starlark.Thread) context.Context {
	if ctx, ok := thread.Local(ctxkey).(context.Context); ok {
		return ctx
	}
	return context.Background()
}

// Resource is a starlark.Value that requires close handling.
type Resource interface {
	starlark.Value
	io.Closer
}

const rsckey = "resources"

func AddResource(thread *starlark.Thread, rsc Resource) {
	if rscs, ok := thread.Local(rsckey).(*[]Resource); ok {
		*rscs = append(*rscs, rsc)
	}
	thread.SetLocal(rsckey, &[]Resource{rsc})
}

func Resources(thread *starlark.Thread) []Resource {
	if rscs, ok := thread.Local(rsckey).(*[]Resource); ok {
		return *rscs
	}
	return nil
}

func CloseResources(thread *starlark.Thread) error {
	rscs := Resources(thread)
	for _, rsc := range rscs {
		if err := rsc.Close(); err != nil {
			return err
		}
	}
	return nil
}
