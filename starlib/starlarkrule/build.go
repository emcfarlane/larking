// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Laze is a task scheduler inspired by Bazel and the Go build tool.
// https://github.com/golang/go/blob/master/src/cmd/go/internal/work/action.go
package starlarkrule

import (
	"container/heap"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"time"

	//"github.com/emcfarlane/larking/starlib"
	"github.com/emcfarlane/larking/starlib/starlarkstruct"
	"github.com/emcfarlane/larking/starlib/starlarkthread"
	"github.com/go-logr/logr"
	"go.starlark.net/starlark"
	"gocloud.dev/blob"
)

// An Action represents a single action in the action graph.
type Action struct {
	*Label // Label is the unique key for an action.

	Deps []*Action // Actions this action depends on.

	Func ImplFunc // Implementation is run when built.

	triggers []*Action // reverse of deps
	pending  int       // number of actions pending
	priority int       // relative execution priority

	// Results
	Value     []*AttrArgs //starlark.Value // caller values
	Error     error       // caller error
	Failed    bool        // whether the action failed
	TimeReady time.Time
	TimeDone  time.Time
}

func (a *Action) String() string { return "action(...)" }
func (a *Action) Type() string   { return "action" }

// Get provides lookup of a key *Attrs to an *AttrArgs if found.
func (a *Action) Get(k starlark.Value) (v starlark.Value, found bool, err error) {
	key, ok := k.(*Attrs)
	if !ok {
		err = fmt.Errorf("invalid key type: %s", k.Type())
		return
	}
	for _, args := range a.Value {
		v = args
		attrs := args.Attrs()

		found, err = starlark.Equal(key, attrs)
		if found || err != nil {
			return
		}
	}
	return starlark.None, false, nil
}

// FailureErr is a DFS on the failed action, returns nil if not failed.
func (a *Action) FailureErr() error {
	if !a.Failed {
		return nil
	}
	if a.Error != nil {
		return a.Error
	}
	for _, a := range a.Deps {
		if err := a.FailureErr(); err != nil {
			return err
		}
	}
	// TODO: panic?
	return fmt.Errorf("unknown failure: %s", a.Key())
}

// An actionQueue is a priority queue of actions.
type actionQueue []*Action

// Implement heap.Interface
func (q *actionQueue) Len() int           { return len(*q) }
func (q *actionQueue) Swap(i, j int)      { (*q)[i], (*q)[j] = (*q)[j], (*q)[i] }
func (q *actionQueue) Less(i, j int) bool { return (*q)[i].priority < (*q)[j].priority }
func (q *actionQueue) Push(x interface{}) { *q = append(*q, x.(*Action)) }
func (q *actionQueue) Pop() interface{} {
	n := len(*q) - 1
	x := (*q)[n]
	*q = (*q)[:n]
	return x
}

func (q *actionQueue) push(a *Action) {
	a.TimeReady = time.Now()
	heap.Push(q, a)
}

func (q *actionQueue) pop() *Action {
	return heap.Pop(q).(*Action)
}

// A Builder holds global state about a build.
type Builder struct {
	//opts builderOptions
	//loader    *starlib.Loader
	//resources *starlarkthread.ResourceStore // resources

	dir *Label // directory
	//tmpDir string // temporary directory TODO: caching tmp dir?

	actionCache map[string]*Action // a cache of already-constructed actions
	targetCache map[string]*Target // a cache of created targets
	moduleCache map[string]bool    // a cache of modules

}

func NewBuilder(l *Label) (*Builder, error) {
	return &Builder{
		dir: l,
	}, nil
}

func (b *Builder) addAction(action *Action) (*Action, error) {
	labelURL := action.Label.String()
	if _, ok := b.actionCache[labelURL]; ok {
		return nil, fmt.Errorf("duplicate action: %s", labelURL)
	}
	if b.actionCache == nil {
		b.actionCache = make(map[string]*Action)
	}
	b.actionCache[labelURL] = action
	return action, nil
}

func (b *Builder) RegisterTarget(thread *starlark.Thread, target *Target) error {
	ctx := starlarkthread.GetContext(thread)
	log := logr.FromContextOrDiscard(ctx)

	// We are in a dir/BUILD.star file
	// Create the target name dir:name
	labelURL := target.label.String()

	if _, ok := b.targetCache[labelURL]; ok {
		return fmt.Errorf("duplicate target: %s", labelURL)
	}
	if b.targetCache == nil {
		b.targetCache = make(map[string]*Target)
	}
	b.targetCache[labelURL] = target

	bktURL := target.label.BucketURL()
	key := target.label.Key()
	log.Info("registered target", "bkt", bktURL, "key", key)
	return nil
}

type ImplFunc func(thread *starlark.Thread) ([]*AttrArgs, error)

func makeDefaultImpl(label *Label) ImplFunc {
	return func(thread *starlark.Thread) ([]*AttrArgs, error) {
		ctx := starlarkthread.GetContext(thread)
		log := logr.FromContextOrDiscard(ctx)

		key := label.Key()
		bktURL := label.BucketURL()
		log.Info("running default impl", "bkt", bktURL, "key", key)
		// TODO: pool bkts.
		bkt, err := blob.OpenBucket(ctx, bktURL)
		if err != nil {
			return nil, err
		}
		defer bkt.Close()

		ok, err := bkt.Exists(ctx, key)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("not exists: %v", label)
		}

		files := []starlark.Value{label}

		source, err := ParseLabel(thread.Name)
		if err != nil {
			return nil, err
		}

		args, err := DefaultInfo.MakeArgs(
			source,
			[]starlark.Tuple{{
				starlark.String("files"),
				starlark.NewList(files),
			}},
		)
		if err != nil {
			return nil, err
		}

		return []*AttrArgs{args}, nil
	}
}

func (b *Builder) createAction(thread *starlark.Thread, label *Label) (*Action, error) {
	ctx := starlarkthread.GetContext(thread)
	log := logr.FromContextOrDiscard(ctx)

	// TODO: validate URL type
	// TODO: label needs to be cleaned...
	u := label.String()
	if action, ok := b.actionCache[u]; ok {
		return action, nil
	}

	labelKey := label.Key()
	dir := path.Dir(labelKey)
	labelURL := label.String()
	bktURL := label.BucketURL()

	log.Info("creating action", "bkt", bktURL, "key", labelKey)

	moduleKey := path.Join(dir, "BUILD.star")
	mod, err := label.Parse(moduleKey)
	if err != nil {
		return nil, err
	}
	moduleURL := mod.String()

	// Load module.
	if ok := b.moduleCache[moduleURL]; !ok { //&& exists(moduleKey) {
		log.Info("loading module", "bkt", bktURL, "key", moduleKey)

		_, err := thread.Load(thread, moduleKey)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				log.Error(err, "failed to load", "key", moduleKey)
				return nil, err
			}
			// ignore not found

		} else {
			// rule will inject the value?
			//for key, val := range d {
			//	fmt.Println(" - ", key, val)
			//}
			if b.moduleCache == nil {
				b.moduleCache = make(map[string]bool)
			}
			b.moduleCache[moduleURL] = true
		}
	}

	// Load rule, or file.
	t, ok := b.targetCache[labelURL]
	if !ok {
		cleanURL := label.CleanURL()
		t, ok = b.targetCache[cleanURL]
		if !ok {
			log.Info("unknown label target", "bkt", bktURL, "key", labelKey)
			return b.addAction(&Action{
				Label: label,
				Deps:  nil,
				Func:  makeDefaultImpl(label),
			})
		}

		kvs, err := label.KeyArgs()
		if err != nil {
			log.Error(err, "invalid key args")
			return nil, err
		}

		t = t.Clone()

		// Parse query params, override args.
		if err := t.SetQuery(kvs); err != nil {
			log.Error(err, "failed to set key args")
			return nil, err
		}

		log.Info("registered query target", "bkt", bktURL, "key", labelKey)
		b.targetCache[labelURL] = t

	}
	log.Info("found label target", "bkt", bktURL, "key", labelKey)

	// TODO: caching the ins & outs?
	// should caching be done on the action execution?

	// Find arg deps as attributes and resolve args to targets.
	args := t.Args()
	rule := t.Rule()
	attrs := rule.Attrs()

	n := args.Len()
	deps := make([]*Action, 0, n/2)
	createAction := func(arg starlark.Value) (*Action, error) {
		l, err := AsLabel(arg)
		if err != nil {
			return nil, err
		}
		action, err := b.createAction(thread, l)
		if err != nil {
			return nil, fmt.Errorf("create action: %w", err)
		}
		deps = append(deps, action)
		return action, nil
	}
	for i := 0; i < n; i++ {
		key, arg := args.KeyIndex(i)
		attr, _ := attrs.Get(key)

		switch attr.Typ {
		case AttrTypeLabel:
			action, err := createAction(arg)
			if err != nil {
				return nil, err
			}
			if err := args.SetField(key, action); err != nil {
				return nil, err
			}

		case AttrTypeLabelList:
			v := arg.(starlark.Indexable)
			elems := make([]starlark.Value, v.Len())
			for i, n := 0, v.Len(); i < n; i++ {
				x := v.Index(i)
				action, err := createAction(x)
				if err != nil {
					return nil, err
				}
				elems[i] = action
			}
			if err := args.SetField(key, starlark.NewList(elems)); err != nil {
				return nil, err
			}

		case AttrTypeLabelKeyedStringDict:
			panic("TODO")

		default:
			continue
		}
	}

	ruleCtx := starlarkstruct.FromKeyValues(
		starlark.String("ctx"),
		"actions", Actions(),
		"attrs", t.Args(),
		"build_dir", starlark.String(dir),
		"build_file", starlark.String(moduleKey),
		"dir", starlark.String(b.dir.Key()),
		"label", label,
		//"outs", starext.MakeBuiltin("ctx.outs", rule.Outs().MakeAttrs),
	)
	ruleCtx.Freeze()

	return b.addAction(&Action{
		Label: label,
		Deps:  deps,
		Func: func(thread *starlark.Thread) ([]*AttrArgs, error) {
			args := starlark.Tuple{ruleCtx}
			val, err := starlark.Call(thread, rule.Impl(), args, nil)
			if err != nil {
				return nil, err
			}

			provides := rule.Provides()

			var results []*AttrArgs
			switch v := val.(type) {
			case *starlark.List:

				results = make([]*AttrArgs, v.Len())
				for i, n := 0, v.Len(); i < n; i++ {
					args, ok := v.Index(i).(*AttrArgs)
					if !ok {
						return nil, fmt.Errorf(
							"unexpect value in list [%d] %s",
							i, v.Index(i).Type(),
						)
					}
					attrs := args.Attrs()
					if _, err := provides.Delete(attrs); err != nil {
						panic(err)
					}
					results[i] = args
				}

			case starlark.NoneType:
				// pass

			default:
				return nil, fmt.Errorf("unknown return type: %s", val.Type())
			}

			if provides.Len() > 0 {
				var buf strings.Builder
				iter := provides.Iterate()
				var (
					p     starlark.Value
					first bool
				)
				for iter.Next(&p) {
					if !first {
						buf.WriteString(", ")
					}
					attrs := p.(*Attrs)
					buf.WriteString(attrs.String())
					first = true
				}
				iter.Done()
				return nil, fmt.Errorf("missing: %v", buf.String())
			}
			return results, nil
		},
	})
}

//// TODO: caching with tmp dir.
//func (b *Builder) init(ctx context.Context) error {
//	//tmpDir, err := ioutil.TempDir("", "laze")
//	//if err != nil {
//	//	return err
//	//}
//	//b.tmpDir = tmpDir
//	return nil
//}
//
//func (b *Builder) cleanup() error {
//	if b.tmpDir != "" {
//		fmt.Println("cleanup", b.tmpDir)
//		// return os.RemoveAll(b.tmpDir)
//	}
//	return nil
//	//if b.WorkDir != "" {
//	//	start := time.Now()
//	//	for {
//	//		err := os.RemoveAll(b.WorkDir)
//	//		if err == nil {
//	//			break
//	//		}
//	//		// On some configurations of Windows, directories containing executable
//	//		// files may be locked for a while after the executable exits (perhaps
//	//		// due to antivirus scans?). It's probably worth a little extra latency
//	//		// on exit to avoid filling up the user's temporary directory with leaked
//	//		// files. (See golang.org/issue/30789.)
//	//		if runtime.GOOS != "windows" || time.Since(start) >= 500*time.Millisecond {
//	//			return fmt.Errorf("failed to remove work dir: %v", err)
//	//		}
//	//		time.Sleep(5 * time.Millisecond)
//	//	}
//	//}
//	//return nil
//}

func (b *Builder) Build(thread *starlark.Thread, label *Label) (*Action, error) {
	setBuilder(thread, b)

	// create action
	root, err := b.createAction(thread, label)
	if err != nil {
		return nil, err
	}
	return root, nil
}

// actionList returns the list of actions in the dag rooted at root
// as visited in a depth-first post-order traversal.
func actionList(root *Action) []*Action {
	seen := map[*Action]bool{}
	all := []*Action{}
	var walk func(*Action)
	walk = func(a *Action) {
		if seen[a] {
			return
		}
		seen[a] = true
		for _, a1 := range a.Deps {
			walk(a1)
		}
		all = append(all, a)
	}
	walk(root)
	return all
}

func (b *Builder) Run(root *Action, threads ...*starlark.Thread) {
	if len(threads) == 0 {
		panic("missing threads")
	}

	// Build list of all actions, assigning depth-first post-order priority.
	all := actionList(root)
	for i, a := range all {
		a.priority = i
	}

	var (
		readyN int
		ready  actionQueue
	)

	// Initialize per-action execution state.
	for _, a := range all {
		for _, a1 := range a.Deps {
			a1.triggers = append(a1.triggers, a)
		}
		a.pending = len(a.Deps)
		if a.pending == 0 {
			ready.push(a)
			readyN++
		}
	}

	// Now we have the list of actions lets run them...
	par := len(threads)
	jobs := make(chan *Action, par)
	done := make(chan *Action, par)
	workerN := par
	for i := 0; i < par; i++ {
		thread := threads[i]
		ctx := starlarkthread.GetContext(thread)
		log := logr.FromContextOrDiscard(ctx)

		go func() {
			for a := range jobs {

				// Run job.
				var value []*AttrArgs
				var err error

				if a.Func != nil && !a.Failed {
					log.Info("running action", "key", a.Key())
					thread.Name = a.Label.String()
					value, err = a.Func(thread)
					thread.Name = ""
					log.Info("completed action", "key", a.Key())
				}
				if err != nil {
					log.Error(err, "action failed", "key", a.Key())
					a.Failed = true
					a.Error = err
				}
				a.Value = value
				a.TimeDone = time.Now()
				done <- a
			}
		}()
	}
	defer close(jobs)

	for i := len(all); i > 0; i-- {
		// Send ready actions to available workers via the jobs queue.
		for readyN > 0 && workerN > 0 {
			jobs <- ready.pop()
			readyN--
			workerN--
		}

		// Wait for completed actions via the done queue.
		a := <-done
		workerN++

		for _, a0 := range a.triggers {
			if a.Failed {
				a0.Failed = true
			}
			if a0.pending--; a0.pending == 0 {
				ready.push(a0)
				readyN++
			}
		}
	}
}
