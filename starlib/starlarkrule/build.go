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

	"github.com/go-logr/logr"
	"go.starlark.net/starlark"

	"larking.io/starlib/starlarkthread"
)

// An Action represents a single action in the action graph.
type Action struct {
	Target *Target   // Target
	Deps   []*Action // Actions this action depends on.

	triggers []*Action // reverse of deps
	pending  int       // number of actions pending
	priority int       // relative execution priority

	// Results
	Values    []starlark.Value
	Error     error // caller error
	Failed    bool  // whether the action failed
	ReadyTime time.Time
	DoneTime  time.Time
}

func (a *Action) String() string {
	buf := new(strings.Builder)
	buf.WriteString(a.Type())
	buf.WriteByte('(')
	buf.WriteString(a.Target.String())
	buf.WriteByte(')')
	return buf.String()

}
func (a *Action) Type() string { return "action" }

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
	return fmt.Errorf("unknown failure: %s", a.Target.String())
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
	a.ReadyTime = time.Now()
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

func SetBuilder(thread *starlark.Thread, builder *Builder) {
	thread.SetLocal(bldKey, builder)
}
func GetBuilder(thread *starlark.Thread) (*Builder, error) {
	if bld, ok := thread.Local(bldKey).(*Builder); ok {
		return bld, nil
	}
	return nil, fmt.Errorf("missing builder")
}

func NewBuilder(l *Label) (*Builder, error) {
	return &Builder{
		dir: l,
	}, nil
}

func (b *Builder) addAction(action *Action) (*Action, error) {
	labelURL := action.Target.Label.String()
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
	labelURL := target.Label.String()

	if _, ok := b.targetCache[labelURL]; ok {
		return fmt.Errorf("duplicate target: %s", labelURL)
	}
	if b.targetCache == nil {
		b.targetCache = make(map[string]*Target)
	}
	b.targetCache[labelURL] = target

	bktURL := target.Label.BucketURL()
	key := target.Label.Key()
	log.Info("registered target", "bkt", bktURL, "key", key)
	return nil
}

//func makeDefaultImpl(label *Label) ImplFunc {
//	return func(thread *starlark.Thread, kwargs []starlark.Tuple) (starlark.Value, error) {
//		ctx := starlarkthread.GetContext(thread)
//		log := logr.FromContextOrDiscard(ctx)
//
//		key := label.Key()
//		bktURL := label.BucketURL()
//		log.Info("running default impl", "bkt", bktURL, "key", key)
//		// TODO: pool bkts.
//		bkt, err := blob.OpenBucket(ctx, bktURL)
//		if err != nil {
//			return nil, err
//		}
//		defer bkt.Close()
//
//		ok, err := bkt.Exists(ctx, key)
//		if err != nil {
//			return nil, err
//		}
//		if !ok {
//			return nil, fmt.Errorf("not exists: %v", label)
//		}
//
//		files := []starlark.Value{label}
//
//		source, err := ParseLabel(thread.Name)
//		if err != nil {
//			return nil, err
//		}
//
//		args, err := DefaultInfo.MakeArgs(
//			source,
//			[]starlark.Tuple{{
//				starlark.String("files"),
//				starlark.NewList(files),
//			}},
//		)
//		if err != nil {
//			return nil, err
//		}
//
//		return []*AttrArgs{args}, nil
//	}
//}

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
			return nil, fmt.Errorf("unknown label target: %q", labelURL)
			//return b.addAction(&Action{
			//	Target: t,
			//	Deps:   nil,
			//	Func:   makeDefaultImpl(label),
			//})
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

	var deps []*Action
	for _, depLabel := range t.Kwargs.Deps {
		depAction, err := b.createAction(thread, depLabel)
		if err != nil {
			return nil, err
		}
		deps = append(deps, depAction)
	}

	//// Find arg deps as attributes and resolve args to targets.
	//args := t.Args()
	//rule := t.Rule()
	//attrs := rule.Attrs()

	//n := args.Len()
	//deps := make([]*Action, 0, n/2)
	//createAction := func(arg starlark.Value) (*Action, error) {
	//	l, err := AsLabel(arg)
	//	if err != nil {
	//		return nil, err
	//	}
	//	action, err := b.createAction(thread, l)
	//	if err != nil {
	//		return nil, fmt.Errorf("create action: %w", err)
	//	}
	//	deps = append(deps, action)
	//	return action, nil
	//}
	//for i := 0; i < n; i++ {
	//	key, arg := args.KeyIndex(i)
	//	attr, _ := attrs.Get(key)

	//	switch attr.Typ {
	//	case AttrTypeLabel:
	//		action, err := createAction(arg)
	//		if err != nil {
	//			return nil, err
	//		}
	//		if err := args.SetField(key, action); err != nil {
	//			return nil, err
	//		}

	//	case AttrTypeLabelList:
	//		v := arg.(starlark.Indexable)
	//		elems := make([]starlark.Value, v.Len())
	//		for i, n := 0, v.Len(); i < n; i++ {
	//			x := v.Index(i)
	//			action, err := createAction(x)
	//			if err != nil {
	//				return nil, err
	//			}
	//			elems[i] = action
	//		}
	//		if err := args.SetField(key, starlark.NewList(elems)); err != nil {
	//			return nil, err
	//		}

	//	case AttrTypeLabelKeyedStringDict:
	//		panic("TODO")

	//	default:
	//		continue
	//	}
	//}

	//ruleCtx := starlarkstruct.FromKeyValues(
	//	starlark.String("ctx"),
	//	//"actions", Actions(),
	//	//"attrs", t.Args(),
	//	"build_dir", starlark.String(dir),
	//	"build_file", starlark.String(moduleKey),
	//	"dir", starlark.String(b.dir.Key()),
	//	"label", label,
	//	//"outs", starext.MakeBuiltin("ctx.outs", rule.Outs().MakeAttrs),
	//)
	//ruleCtx.Freeze()

	return b.addAction(&Action{
		Target: t,
		Deps:   deps,
		//Func: func(thread *starlark.Thread, kwargs []starlark.Tuple) (starlark.Value, error) {
		//	return starlark.Call(thread, t.Rule.impl, nil, kwargs)
		//},
	})
}

//// TODO: caching with tmp dir.
//func (b *Builder) init(ctx context.Context) error {
//	//tmpDir, err := os.CreateTemp("", "laze")
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
	SetBuilder(thread, b)

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

func (b *Builder) RunAction(thread *starlark.Thread, a *Action) {
	ctx := starlarkthread.GetContext(thread)
	log := logr.FromContextOrDiscard(ctx)

	handle := func(a *Action) error {
		// Run job.
		kwargs, err := a.Target.Kwargs.Resolve(a.Deps)
		if err != nil {
			log.Error(err, "failed to resolve kwargs", "key", a.Target.Label.Key())
			return err
		}
		impl := a.Target.Rule.Impl()
		if a.Failed || impl == nil {
			log.Info("ignoring action", "key", a.Target.Label.Key())
			return nil
		}

		log.Info("running action", "key", a.Target.Label.Key())
		thread.Name = a.Target.Label.String()
		value, err := starlark.Call(thread, impl, nil, kwargs)
		thread.Name = ""
		log.Info("completed action", "key", a.Target.Label.Key())
		if err != nil {
			return err
		}

		switch x := value.(type) {
		case *starlark.List:
			n := x.Len()
			lst := make([]starlark.Value, 0, n)

			//lookup := make(map[protoreflect.FullName]*Attr)
			//for _, attr := range a.Target.Rule.provides {
			//	lookup[attr.FullName] = attr
			//}
			x.Freeze()

			for i := 0; i < n; i++ {
				val := x.Index(i)
				key, err := toType(value)
				if err != nil {
					return err
				}

				if _, ok := a.Target.Rule.providesMap[key]; !ok {
					log.Info("missing attr type")
					return fmt.Errorf("no provider for type: %q", key)
				}
				lst = append(lst, val)
			}

			a.Values = lst
			return nil

		case starlark.NoneType:
			// pass
			return nil
		default:
			return fmt.Errorf("invalid return type: %q", value.Type())
		}
	}
	if err := handle(a); err != nil {
		log.Error(err, "action failed", "key", a.Target.Label.Key())
		a.Failed = true
		a.Error = err
	}
	a.DoneTime = time.Now()
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
		go func() {
			for a := range jobs {
				b.RunAction(thread, a)
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
