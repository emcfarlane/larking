// Implements a laze scheduler.
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"

	_ "github.com/emcfarlane/larking/cmd/internal/bindings"
	"github.com/emcfarlane/larking/laze"
	"github.com/pkg/browser"
)

var (
	flagWeb = flag.Bool("web", false, "Opens in the browser")
)

func run(ctx context.Context) error {
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		return fmt.Errorf("missing label")
	}

	label := args[len(args)-1]
	args = args[:len(args)-1]

	b, err := laze.NewBuilder("") // TODO: configuration?
	if err != nil {
		return err
	}

	a, err := b.Build(ctx, args, label)
	if err != nil {
		return err
	}

	if *flagWeb {
		addr := "localhost:0"
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return err
		}
		defer ln.Close()

		u := url.URL{
			Scheme: "http",
			Host:   ln.Addr().String(),
			Path:   "/graph/" + label,
		}
		go func() {
			if err := browser.OpenURL(u.String()); err != nil {
				fmt.Println(err)
			}
		}()

		fmt.Println(ln.Addr())
		return b.Serve(ln)
	}

	b.Run(ctx, a)

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
