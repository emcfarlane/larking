// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlib

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"sync"

	"github.com/emcfarlane/larking/starlarkblob"
	"github.com/emcfarlane/larking/starlarkdocstore"
	"github.com/emcfarlane/larking/starlarkerrors"
	"github.com/emcfarlane/larking/starlarknethttp"
	"github.com/emcfarlane/larking/starlarkpubsub"
	"github.com/emcfarlane/larking/starlarkruntimevar"
	"github.com/emcfarlane/larking/starlarksql"
	"github.com/emcfarlane/starlarkassert"
	"github.com/emcfarlane/starlarkproto"
	starlarkjson "go.starlark.net/lib/json"
	starlarkmath "go.starlark.net/lib/math"
	starlarktime "go.starlark.net/lib/time"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// TODO: load syntax
// git modules...

// call is an in-flight or completed load call
type call struct {
	wg  sync.WaitGroup
	val starlark.StringDict
	err error

	// callers are idle threads waiting on the call
	callers map[*starlark.Thread]bool
}

type Loader struct {
	mu sync.Mutex       // protects m
	m  map[string]*call // lazily initialized

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
		c.callers[thread] = true
		l.mu.Unlock()
		c.wg.Wait()
		return c.val, c.err
	}
	c := new(call)
	c.wg.Add(1)
	c.callers[thread] = true
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

func (l *Loader) load(thread *starlark.Thread, module string) (starlark.StringDict, error) {
	// TODO
	return nil, os.ErrNotExist
}

var (
	stdOnce sync.Once
	stdLib  map[string]starlark.StringDict
)

func stdOnceLoad(thread *starlark.Thread) error {
	assert, err := starlarkassert.LoadAssertModule(thread)
	if err != nil {
		return err
	}
	store := map[string]starlark.StringDict{
		"assert.star": assert,
	}
	modules := []*starlarkstruct.Module{
		starlarkblob.NewModule(),
		starlarkdocstore.NewModule(),
		starlarkerrors.NewModule(),
		starlarknethttp.NewModule(),
		starlarkpubsub.NewModule(),
		starlarkruntimevar.NewModule(),
		starlarksql.NewModule(),
		starlarkproto.NewModule(protoregistry.GlobalFiles),

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
		store[module.Name+".star"] = dict
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
