// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlib

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"sync"

	"github.com/emcfarlane/larking/starlarkblob"
	"github.com/emcfarlane/larking/starlarkdocstore"
	"github.com/emcfarlane/larking/starlarkerrors"
	"github.com/emcfarlane/larking/starlarkhttp"
	"github.com/emcfarlane/larking/starlarkproto"
	"github.com/emcfarlane/larking/starlarkpubsub"
	"github.com/emcfarlane/larking/starlarkruntimevar"
	"github.com/emcfarlane/larking/starlarksql"
	"github.com/emcfarlane/larking/starlarkstruct"
	"github.com/emcfarlane/larking/starlarkthread"
	starlarkjson "go.starlark.net/lib/json"
	starlarkmath "go.starlark.net/lib/math"
	starlarktime "go.starlark.net/lib/time"
	"go.starlark.net/starlark"
	"gocloud.dev/blob"
)

var (
	stdOnce sync.Once
	stdLib  map[string]starlark.StringDict
)

func stdOnceLoad(_ *starlark.Thread) error {
	modules := []*starlarkstruct.Module{
		starlarkblob.NewModule(),
		starlarkdocstore.NewModule(),
		starlarkerrors.NewModule(),
		starlarkhttp.NewModule(),
		starlarkpubsub.NewModule(),
		starlarkruntimevar.NewModule(),
		starlarksql.NewModule(),
		starlarkproto.NewModule(),

		// TODO: starlarkgrpc...

		// starlark native modules
		starlarkjson.Module,
		starlarkmath.Module,
		starlarktime.Module,
	}
	for _, module := range modules {
		dict := make(starlark.StringDict, len(module.Members)+1)
		for key, val := range module.Members {
			dict[key] = val
		}
		dict[module.Name] = module
		stdLib[module.Name+".star"] = dict
	}
	return nil
}

func StdLoad(thread *starlark.Thread, module string) (starlark.StringDict, error) {
	var lderr error
	if stdOnce.Do(func() {
		lderr = stdOnceLoad(thread)
	}); lderr != nil {
		return nil, lderr
	}
	if e, ok := stdLib[module]; ok {
		return e, nil
	}
	return nil, os.ErrNotExist
}

// call is an in-flight or completed load call
type call struct {
	wg  sync.WaitGroup
	val starlark.StringDict
	err error

	// callers are idle threads waiting on the call
	callers map[*starlark.Thread]bool
}

type Loader struct {
	mu   sync.Mutex       // protects m
	m    map[string]*call // lazily initialized
	bkts map[string]*blob.Bucket

	//
}

func NewLoader() *Loader {
	return &Loader{}
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

func (l *Loader) Load(thread *starlark.Thread, module string) (starlark.StringDict, error) {
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

func (l *Loader) loadBucket(ctx context.Context, bktURL string) (*blob.Bucket, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if bkt, ok := l.bkts[bktURL]; ok {
		return bkt, nil
	}

	bkt, err := blob.OpenBucket(ctx, bktURL)
	if err != nil {
		return nil, err
	}
	l.bkts[bktURL] = bkt
	return bkt, nil
}

func (l *Loader) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	var firstErr error
	for _, bkt := range l.bkts {
		if err := bkt.Close(); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (l *Loader) load(thread *starlark.Thread, module string) (starlark.StringDict, error) {
	ctx := starlarkthread.Context(thread)

	v, err := StdLoad(thread, module)
	if err == nil {
		return v, nil
	}
	if err != os.ErrNotExist {
		return nil, err
	}

	bktURL, key, err := resolveModuleURL(thread.Name, module)
	if err != nil {
		return nil, err
	}

	bkt, err := l.loadBucket(ctx, bktURL)
	if err != nil {
		return nil, err
	}

	src, err := bkt.ReadAll(ctx, key)
	if err != nil {
		return nil, err
	}

	oldName, newName := thread.Name, bktURL
	thread.Name = newName
	defer func() { thread.Name = oldName }()

	v, err = starlark.ExecFile(thread, key, src, nil)
	if err != nil {
		return nil, err
	}
	return v, nil
}
