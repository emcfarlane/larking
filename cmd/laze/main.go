// Implements a laze scheduler.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/emcfarlane/larking/laze"
)

func run(ctx context.Context) error {
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		return fmt.Errorf("missing label")
	}

	label := args[len(args)-1]
	args = args[:len(args)-1]

	b := laze.NewBuilder("") // TODO: configuration?

	a, err := b.Build(ctx, args, label)
	if err != nil {
		return err
	}

	// Report error on failed actions.
	if err := a.FailureErr(); err != nil {
		return err
	}
	return nil
}

func main() {
	ctx := context.Background()
	if err := run(ctx); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
