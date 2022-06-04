// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlib

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"runtime/debug"
	"sync"

	"github.com/emcfarlane/larking/starlib/encoding/starlarkproto"
	"github.com/emcfarlane/larking/starlib/net/starlarkhttp"
	"github.com/emcfarlane/larking/starlib/net/starlarkopenapi"
	"github.com/go-logr/logr"

	//"github.com/emcfarlane/larking/starlib/net/starlarkgrpc"
	"github.com/emcfarlane/larking/starlib/starlarkblob"
	"github.com/emcfarlane/larking/starlib/starlarkdocstore"
	"github.com/emcfarlane/larking/starlib/starlarkerrors"
	"github.com/emcfarlane/larking/starlib/starlarkpubsub"
	"github.com/emcfarlane/larking/starlib/starlarkrule"
	"github.com/emcfarlane/larking/starlib/starlarkruntimevar"
	"github.com/emcfarlane/larking/starlib/starlarksql"
	"github.com/emcfarlane/larking/starlib/starlarkstruct"
	"github.com/emcfarlane/larking/starlib/starlarkthread"
	starlarkjson "go.starlark.net/lib/json"
	starlarkmath "go.starlark.net/lib/math"
	starlarktime "go.starlark.net/lib/time"
	"go.starlark.net/starlark"
	"gocloud.dev/blob"
)

// content holds our static web server content.
//go:embed rules/*
var local embed.FS

func makeDict(module *starlarkstruct.Module) starlark.StringDict {
	dict := make(starlark.StringDict, len(module.Members)+1)
	for key, val := range module.Members {
		dict[key] = val
	}
	// Add module if no module name.
	if _, ok := dict[module.Name]; !ok {
		dict[module.Name] = module
	}
	return dict
}

var (
	stdLibMu sync.Mutex
	stdLib   = map[string]starlark.StringDict{
		//"archive/container.star":           makeDict(starlarkcontainer.NewModule()),
		//"archive/tar.star":           makeDict(starlarktar.NewModule()),
		//"archive/zip.star":           makeDict(starlarkzip.NewModule()),

		"blob.star":           makeDict(starlarkblob.NewModule()),
		"docstore.star":       makeDict(starlarkdocstore.NewModule()),
		"encoding/json.star":  makeDict(starlarkjson.Module), // starlark
		"encoding/proto.star": makeDict(starlarkproto.NewModule()),
		"errors.star":         makeDict(starlarkerrors.NewModule()),
		"math.star":           makeDict(starlarkmath.Module), // starlark
		"net/http.star":       makeDict(starlarkhttp.NewModule()),
		"net/openapi.star":    makeDict(starlarkopenapi.NewModule()),
		"pubsub.star":         makeDict(starlarkpubsub.NewModule()),
		"runtimevar.star":     makeDict(starlarkruntimevar.NewModule()),
		"sql.star":            makeDict(starlarksql.NewModule()),
		"time.star":           makeDict(starlarktime.Module), // starlark
		"thread.star":         makeDict(starlarkthread.NewModule()),
		"rule.star":           makeDict(starlarkrule.NewModule()),
		//"net/grpc.star": makeDict(starlarkgrpc.NewModule()),
	}
)

// StdLoad loads files from the standard library.
func (l *Loader) StdLoad(thread *starlark.Thread, module string) (starlark.StringDict, error) {
	stdLibMu.Lock()
	if e, ok := stdLib[module]; ok {
		stdLibMu.Unlock()
		return e, nil
	}
	stdLibMu.Unlock()

	// Load and eval the file.
	src, err := local.ReadFile(module)
	if err != nil {
		return nil, err
	}
	v, err := starlark.ExecFile(thread, module, src, l.globals)
	if err != nil {
		return nil, fmt.Errorf("exec file: %v", err)
	}

	stdLibMu.Lock()
	stdLib[module] = v // cache
	stdLibMu.Unlock()
	return v, nil
}

// call is an in-flight or completed load call
type call struct {
	wg  sync.WaitGroup
	val starlark.StringDict
	err error

	// callers are idle threads waiting on the call
	callers map[*starlark.Thread]bool
}

// Loader is a cloid.Blob backed loader. It uses thread.Name to figure out the
// current bucket and module.
type Loader struct {
	mu   sync.Mutex       // protects m
	m    map[string]*call // lazily initialized
	bkts map[string]*blob.Bucket

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
	defer log.Info("finsihed loading", "module", module)

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

// LoadBucket opens a bucket from a bucket of buckets.
func (l *Loader) LoadBucket(ctx context.Context, bktURL string) (*blob.Bucket, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if bkt, ok := l.bkts[bktURL]; ok {
		return bkt, nil
	}

	bkt, err := blob.OpenBucket(ctx, bktURL)
	if err != nil {
		return nil, err
	}

	if l.bkts == nil {
		l.bkts = make(map[string]*blob.Bucket)
	}
	l.bkts[bktURL] = bkt
	return bkt, nil
}

// Close open buckets.
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

// LoadSource fetches the source file in bytes from a bucket.
func (l *Loader) LoadSource(ctx context.Context, bktURL string, key string) ([]byte, error) {
	bkt, err := l.LoadBucket(ctx, bktURL)
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
		fmt.Println("error", errors.Is(err, fs.ErrNotExist))
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
