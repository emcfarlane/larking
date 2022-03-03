// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlarkthread

import (
	"context"
	"fmt"
	"io"
	"testing"

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

// ResourceStore is a thread local storage map for adding resources.
type ResourceStore map[Resource]bool // resource whether it's living

// WithResourceStore returns a cleanup function. It is required for
// packages that add resources.
func WithResourceStore(thread *starlark.Thread) func() error {
	store := make(ResourceStore)
	thread.SetLocal(rsckey, store)
	return func() error { return CloseResources(thread) }
}

func AddResource(thread *starlark.Thread, rsc Resource) error {
	// runtime.SetFinalizer?
	//
	store, ok := thread.Local(rsckey).(ResourceStore)
	if !ok {
		return fmt.Errorf("invalid thread resource store")
	}
	store[rsc] = true
	return nil
}

func Resources(thread *starlark.Thread) ResourceStore {
	if store, ok := thread.Local(rsckey).(ResourceStore); ok {
		return store
	}
	return nil
}

func CloseResources(thread *starlark.Thread) (firstErr error) {
	store := Resources(thread)
	for rsc, open := range store {
		if !open {
			continue
		}
		if err := rsc.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		store[rsc] = false
	}
	return
}

// AssertOption implements starlarkassert.TestOption
// Add like so:
//
// 	starlarkassert.RunTests(t, "*.star", globals, AssertOption)
//
func AssertOption(t testing.TB, thread *starlark.Thread) func() {
	close := WithResourceStore(thread)
	return func() {
		if err := close(); err != nil {
			t.Error(err, "failed to close resources")
		}
	}
}
