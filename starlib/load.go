// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlib

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"runtime/debug"
	"sync"

	"github.com/go-logr/logr"
	"larking.io/starlib/starext"

	//"larking.io/starlib/net/starlarkgrpc"

	"go.starlark.net/starlark"
	"larking.io/starlib/starlarkrule"
	"larking.io/starlib/starlarkthread"
)

// call is an in-flight or completed load call
type call struct {
	wg  sync.WaitGroup
	val starlark.StringDict
	err error

	// callers are idle threads waiting on the call
	callers map[*starlark.Thread]bool
}

// Loader is a cloud.Blob backed loader. It uses thread.Name to figure out the
// current bucket and module.
// TODO: document how this works.
type Loader struct {
	starext.Blobs

	mu sync.Mutex       // protects m
	m  map[string]*call // lazily initialized
	//cache map[string]starlark.StringDict

	// Predeclared globals
	globals starlark.StringDict
}

func NewLoader(globals starlark.StringDict) *Loader {
	return &Loader{
		globals: globals,
	}
}

// errCycle indicates the load caused a cycle.
var errCycle = errors.New("cycle in loading module")

// A panicError is an arbitrary value recovered from a panic
// with the stack trace during the execution of given function.
type panicError struct {
	value interface{}
	stack []byte
}

// Error implements error interface.
func (p *panicError) Error() string {
	return fmt.Sprintf("%v\n\n%s", p.value, p.stack)
}
func newPanicError(v interface{}) error {
	stack := debug.Stack()

	// The first line of the stack trace is of the form "goroutine N [status]:"
	// but by the time the panic reaches Do the goroutine may no longer exist
	// and its status will have changed. Trim out the misleading line.
	if line := bytes.IndexByte(stack[:], '\n'); line >= 0 {
		stack = stack[line+1:]
	}
	return &panicError{value: v, stack: stack}
}

// Load checks the standard library before loading from buckets.
func (l *Loader) Load(thread *starlark.Thread, module string) (starlark.StringDict, error) {
	ctx := starlarkthread.GetContext(thread)
	log := logr.FromContextOrDiscard(ctx)

	log.Info("loading module", "module", module)
	defer log.Info("finished loading", "module", module)

	l.mu.Lock()
	if l.m == nil {
		l.m = make(map[string]*call)
	}
	key := module // TODO: hash file contents?
	if c, ok := l.m[key]; ok {
		if c.callers[thread] {
			l.mu.Unlock()
			return nil, errCycle
		}
		l.mu.Unlock()
		c.wg.Wait()
		return c.val, c.err
	}
	c := new(call)
	c.wg.Add(1)
	c.callers = map[*starlark.Thread]bool{thread: true}
	l.m[key] = c
	l.mu.Unlock()

	func() {
		defer func() {
			if r := recover(); r != nil {
				c.err = newPanicError(r)
			}
		}()
		c.val, c.err = l.load(thread, module)
	}()
	c.wg.Done()

	l.mu.Lock()
	delete(l.m, key) // TODO: hash files.
	l.mu.Unlock()

	return c.val, c.err
}

// LoadSource fetches the source file in bytes from a bucket.
func (l *Loader) LoadSource(ctx context.Context, bktURL string, key string) ([]byte, error) {
	bkt, err := l.OpenBucket(ctx, bktURL)
	if err != nil {
		return nil, err
	}
	return bkt.ReadAll(ctx, key)
}

func (l *Loader) load(thread *starlark.Thread, module string) (starlark.StringDict, error) {
	ctx := starlarkthread.GetContext(thread)

	v, err := l.StdLoad(thread, module)
	if err == nil {
		return v, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	label, err := starlarkrule.ParseRelativeLabel(thread.Name, module)
	if err != nil {
		return nil, err
	}

	bktURL := label.BucketURL()
	key := label.Key()

	src, err := l.LoadSource(ctx, bktURL, key)
	if err != nil {
		return nil, err
	}

	oldName, newName := thread.Name, label.String()
	thread.Name = newName
	defer func() { thread.Name = oldName }()

	v, err = starlark.ExecFile(thread, key, src, l.globals)
	if err != nil {
		return nil, err
	}
	return v, nil
}
