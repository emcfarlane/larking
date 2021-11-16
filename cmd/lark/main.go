// lark
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/emcfarlane/larking"
	"github.com/emcfarlane/larking/api"
	"github.com/emcfarlane/larking/starlarkthread"
	"github.com/peterh/liner"
	"go.starlark.net/repl"
	"go.starlark.net/resolve"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
	"google.golang.org/grpc"
)

func init() {
	// TODO: fix this repl issue.
	resolve.LoadBindsGlobally = true
}

func env(key, def string) string {
	if e := os.Getenv(key); e != "" {
		return e
	}
	return def
}

var (
	flagRemote       = flag.String("remote", env("LARK_REMOTE", ""), "Remote server address to execute on.")
	flagHistory      = flag.String("history", env("LARK_HISTORY", ""), "History file.")
	flagAutocomplete = flag.Bool("autocomplete", true, "Enable autocomplete, defaults to true.")
)

type Options struct {
	_            struct{}          // pragma: no unkeyed literals
	HistoryFile  string            // Path to file for storing history
	AutoComplete bool              // Experimental autocompletion
	Remote       api.LarkingClient // Remote thread execution
}

func read(line *liner.State, buf *bytes.Buffer) (*syntax.File, error) {
	buf.Reset()

	// suggest
	suggest := func(line string) string {
		var noSpaces int
		for _, c := range line {
			if c == ' ' {
				noSpaces += 1
			} else {
				break
			}
		}
		if strings.HasSuffix(line, ":") {
			noSpaces += 4
		}
		return strings.Repeat(" ", noSpaces)
	}

	var eof bool
	var previous string
	prompt := ">>> "
	readline := func() ([]byte, error) {
		text := suggest(previous)
		s, err := line.PromptWithSuggestion(prompt, text, -1)
		if err != nil {
			switch err {
			case io.EOF:
				eof = true
			case liner.ErrPromptAborted:
				return []byte("\n"), nil
			}
			return nil, err
		}
		prompt = "... "
		previous = s
		line.AppendHistory(s)
		return []byte(s + "\n"), nil
	}

	f, err := syntax.ParseCompoundStmt("<stdin>", readline)
	if err != nil {
		if eof {
			return nil, io.EOF
		}
		repl.PrintError(err)
		return nil, err
	}
	return f, nil
}

func remote(ctx context.Context, line *liner.State, client api.LarkingClient, autocomplete bool) error {
	stream, err := client.RunOnThread(ctx)
	if err != nil {
		return err
	}

	if autocomplete {
		line.SetCompleter(func(line string) []string {
			return []string{} // TODO
		})
	}

	var buf bytes.Buffer
	for ctx.Err() == nil {
		_, err := read(line, &buf)
		if err != nil {
			if err == io.EOF {
				fmt.Println()
			}
			return err
		}

		cmd := &api.Command{
			Name: "default", // TODO: name?
			Exec: &api.Command_Input{
				Input: buf.String(),
			},
		}
		if err := stream.Send(cmd); err != nil {
			return err // TODO: retry?
		}

		res, err := stream.Recv()
		if err != nil {
			return err // TODO: retry?
		}
		if output := res.GetOutput(); output != nil {
			if output.Output != "" {
				fmt.Println(output.Output)
			}
		}
	}
	return ctx.Err()
}

func local(ctx context.Context, line *liner.State, autocomplete bool) error {
	globals := larking.NewGlobals()

	loader, err := larking.NewLoader()
	if err != nil {
		return err
	}

	thread := &starlark.Thread{
		Name: "<stdin>",
		Load: loader.Load,
	}
	starlarkthread.SetContext(thread, ctx)
	defer starlarkthread.WithResourceStore(thread)()

	if autocomplete {
		c := larking.Completer{globals}
		line.SetCompleter(c.Complete)
	}

	soleExpr := func(f *syntax.File) syntax.Expr {
		if len(f.Stmts) == 1 {
			if stmt, ok := f.Stmts[0].(*syntax.ExprStmt); ok {
				return stmt.X
			}
		}
		return nil
	}

	var buf bytes.Buffer
	for ctx.Err() == nil {
		f, err := read(line, &buf)
		if err != nil {
			return err
		}

		if expr := soleExpr(f); expr != nil {
			// eval
			v, err := starlark.EvalExpr(thread, expr, globals)
			if err != nil {
				repl.PrintError(err)
				return nil
			}

			// print
			if v != starlark.None {
				fmt.Println(v)
			}
		} else if err := starlark.ExecREPLChunk(f, thread, globals); err != nil {
			repl.PrintError(err)
			return nil
		}
	}
	return ctx.Err()
}

func loop(ctx context.Context, options *Options) (err error) {
	line := liner.NewLiner()
	defer line.Close()

	if options.HistoryFile != "" {
		if f, err := os.Open(options.HistoryFile); err == nil {
			if _, err := line.ReadHistory(f); err != nil {
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		}
	}

	if client := options.Remote; client != nil {
		err = remote(ctx, line, client, options.AutoComplete)
	} else {
		err = local(ctx, line, options.AutoComplete)
	}

	if options.HistoryFile != "" {
		f, err := os.Create(options.HistoryFile)
		if err != nil {
			return err
		}
		if _, err := line.WriteHistory(f); err != nil {
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	return
}

func run() (err error) {
	flag.Parse()
	ctx := context.Background()

	var client api.LarkingClient
	if addr := *flagRemote; addr != "" {
		cc, err := grpc.Dial(addr)
		if err != nil {
			return err
		}
		defer cc.Close()

		client = api.NewLarkingClient(cc)
	}

	var history string
	if filename := *flagHistory; filename != "" {
		history = filename
	} else {
		//
		dirname, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		history = filepath.Join(dirname, ".lark_history")
	}

	options := &Options{
		HistoryFile:  history,
		AutoComplete: true,
		Remote:       client,
	}
	if err := loop(ctx, options); err != io.EOF {
		return err
	}
	fmt.Println() // break EOF
	return err

}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
