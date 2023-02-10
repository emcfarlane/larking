package builder

import (
	"context"
	"fmt"
	"runtime"

	"github.com/go-logr/logr"
	"go.starlark.net/starlark"
	"larking.io/starlib"
	"larking.io/starlib/starlarkrule"
	"larking.io/starlib/starlarkthread"
)

type buildOptions struct {
	buildP int
	run    bool
}

var defaultBuildOptions = buildOptions{
	buildP: runtime.GOMAXPROCS(0),
	run:    true,
}

// BuildOption
type BuildOption func(*buildOptions)

func Build(ctx context.Context, label string, opts ...BuildOption) (*starlarkrule.Action, error) {
	bldOpts := defaultBuildOptions
	for _, opt := range opts {
		opt(&bldOpts)
	}
	log := logr.FromContextOrDiscard(ctx)

	dir := "" // todo?
	l, err := starlarkrule.ParseRelativeLabel("file://./?metadata=skip", dir)
	if err != nil {
		return nil, err
	}

	b, err := starlarkrule.NewBuilder(l)
	if err != nil {
		return nil, err
	}

	globals := starlib.NewGlobals()
	loader := starlib.NewLoader(globals)
	resources := starlarkthread.ResourceStore{} // resources
	defer resources.Close()

	bktURL := l.BucketURL()
	threads := make([]*starlark.Thread, bldOpts.buildP)
	for i := 0; i < bldOpts.buildP; i++ {
		thread := &starlark.Thread{
			Name: bktURL,
			Load: loader.Load,
			Print: func(thread *starlark.Thread, msg string) {
				fmt.Println("MSG!", msg)
				log.Info(msg, "name", thread.Name)
			},
		}
		starlarkthread.SetResourceStore(thread, &resources)
		starlarkthread.SetContext(thread, ctx)
		threads[i] = thread
	}
	thread := threads[0]

	l, err = l.Parse(label)
	if err != nil {
		return nil, err
	}

	action, err := b.Build(thread, l)
	if err != nil {
		return nil, err
	}

	if bldOpts.run {
		b.Run(action, threads...)
	}
	return action, nil
}
