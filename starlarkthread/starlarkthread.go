// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlarkthread

import (
	"context"

	"go.starlark.net/starlark"
)

const key = "context"

// SetThreadContext sets the thread context.
func SetContext(thread *starlark.Thread, ctx context.Context) {
	thread.SetLocal(key, ctx)
}

// Context is a simple helper for getting a threads context.
// ctx := starlarkthread.Context(thread)
func Context(thread *starlark.Thread) context.Context {
	if ctx, ok := thread.Local(key).(context.Context); ok {
		return ctx
	}
	return context.Background()
}
