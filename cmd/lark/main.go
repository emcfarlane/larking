// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// lark
package main

import (
	"bytes"
	"context"
	"crypto/x509"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/emcfarlane/larking/apipb/workerpb"
	_ "github.com/emcfarlane/larking/cmd/internal/bindings"
	"github.com/emcfarlane/larking/control"
	"github.com/emcfarlane/larking/starlib"
	"github.com/emcfarlane/larking/starlib/starlarkthread"
	"github.com/emcfarlane/larking/worker"
	"github.com/emcfarlane/starlarkassert"
	"github.com/peterh/liner"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
	"gocloud.dev/blob"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func env(key, def string) string {
	if e := os.Getenv(key); e != "" {
		return e
	}
	return def
}

var (
	flagRemoteAddr   = flag.String("remote", env("LARK_REMOTE", ""), "Remote server address to execute on.")
	flagCacheDir     = flag.String("cache", env("LARK_CACHE", ""), "Cache directory.")
	flagAutocomplete = flag.Bool("autocomplete", true, "Enable autocomplete, defaults to true.")
	flagExecprog     = flag.String("c", "", "Execute program `prog`.")
	flagControlAddr  = flag.String("control", "https://larking.io", "Control server for credentials.")
	flagInsecure     = flag.Bool("insecure", false, "Insecure, disable credentials.")
	flagThread       = flag.String("thread", "", "Thread to run on.")
	flagCreds        = flag.String("credentials", env("CREDENTIALS", ""), "Runtime variable for credentials.")

	// TODO: relative/absolute pathing needs to be resolved...
	flagDir = flag.String("dir", env("LARK_DIR", "file://./?metadata=skip"), "Set the module loading directory")
)

type Options struct {
	_            struct{}              // pragma: no unkeyed literals
	CacheDir     string                // Path to cache directory
	HistoryFile  string                // Path to file for storing history
	AutoComplete bool                  // Experimental autocompletion
	Remote       workerpb.WorkerClient // Remote thread execution
	//RemoteAddr          string // Remote worker address.
	//CredentialsFile string // Path to file for remote credentials
	//Creds map[string]string // Loaded credentials.
	Filename string
	Source   string
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
		out := []byte(s + "\n")
		if _, err := buf.Write(out); err != nil {
			return nil, err
		}
		return out, nil
	}

	f, err := syntax.ParseCompoundStmt("<stdin>", readline)
	if err != nil {
		if eof {
			return nil, io.EOF
		}
		starlib.FprintErr(os.Stderr, err)
		return nil, err
	}
	return f, nil
}

func remote(ctx context.Context, line *liner.State, client workerpb.WorkerClient, autocomplete bool) error {
	stream, err := client.RunOnThread(ctx)
	if err != nil {
		return err
	}

	if autocomplete {
		line.SetCompleter(func(line string) []string {
			if err := stream.SendMsg(&workerpb.Command{
				Exec: &workerpb.Command_Complete{
					Complete: line,
				},
			}); err != nil {
				return nil
			}
			result, err := stream.Recv()
			if err != nil {
				return nil
			}
			if completion := result.GetCompletion(); completion != nil {
				return completion.Completions
			}
			return nil
		})
	}

	var buf bytes.Buffer
	for ctx.Err() == nil {
		_, err := read(line, &buf)
		if err != nil {
			if err == io.EOF {
				return err
			}
			continue
		}

		cmd := &workerpb.Command{
			Name: *flagThread,
			Exec: &workerpb.Command_Input{
				Input: buf.String(),
			},
		}
		if err := stream.Send(cmd); err != nil {
			if err == io.EOF {
				fmt.Fprint(os.Stderr, "eof")
				return err
			}
			starlib.FprintErr(os.Stderr, err)
			continue
		}

		res, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				return err
			}
			starlib.FprintErr(os.Stderr, err)
			return err
		}
		if output := res.GetOutput(); output != nil {
			if output.Output != "" {
				fmt.Println(output.Output)
			}
			if output.Status != nil {
				starlib.FprintStatus(os.Stderr, output.Status)
			}
		}
	}
	return ctx.Err()
}

func printer() func(*starlark.Thread, string) {
	return func(_ *starlark.Thread, msg string) {
		os.Stdout.WriteString(msg + "\n")
	}
}

func local(ctx context.Context, line *liner.State, autocomplete bool) (err error) {
	globals := starlib.NewGlobals()
	loader := starlib.NewLoader(globals)
	defer loader.Close()

	thread := &starlark.Thread{
		Name:  "<stdin>",
		Load:  loader.Load,
		Print: printer(),
	}
	starlarkthread.SetContext(thread, ctx)
	close := starlarkthread.WithResourceStore(thread)
	defer func() {
		if cerr := close(); err == nil {
			err = cerr
		}
	}()

	if autocomplete {
		c := starlib.Completer{StringDict: globals}
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
			if err == io.EOF {
				return err
			}
			continue
		}

		if expr := soleExpr(f); expr != nil {
			// eval
			v, err := starlark.EvalExpr(thread, expr, globals)
			if err != nil {
				starlib.FprintErr(os.Stderr, err)
				continue
			}

			// print
			if v != starlark.None {
				fmt.Println(v)
			}
		} else if err := starlark.ExecREPLChunk(f, thread, globals); err != nil {
			starlib.FprintErr(os.Stderr, err)
			continue
		}
	}
	return ctx.Err()
}

func loop(ctx context.Context, opts *Options) (err error) {
	line := liner.NewLiner()
	defer line.Close()

	if opts.HistoryFile != "" {
		if f, err := os.Open(opts.HistoryFile); err == nil {
			if err != nil {
				return nil
			}
			if _, err := line.ReadHistory(f); err != nil {
				f.Close() //nolint
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		}
	}

	if client := opts.Remote; client != nil {
		err = remote(ctx, line, client, opts.AutoComplete)
	} else {
		err = local(ctx, line, opts.AutoComplete)
	}
	if opts.HistoryFile != "" {
		f, err := os.Create(opts.HistoryFile)
		if err != nil {
			return err
		}
		if _, err := line.WriteHistory(f); err != nil {
			f.Close() //nolint
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	return
}

func loadTransportCredentials(ctx context.Context) (credentials.TransportCredentials, error) {
	if *flagInsecure {
		return insecure.NewCredentials(), nil
	}
	pool, err := x509.SystemCertPool()
	if err != nil {
		return nil, err
	}
	return credentials.NewClientTLSFromCert(pool, ""), nil
}

func createRemoteConn(ctx context.Context, addr string, ctrlClient *control.Client) (*grpc.ClientConn, error) {
	var opts []grpc.DialOption
	if !*flagInsecure {
		if u := *flagCreds; u != "" {
			perRPC, err := control.OpenRPCCredentials(ctx, *flagCreds)
			if err != nil {
				return nil, err
			}
			opts = append(opts, grpc.WithPerRPCCredentials(perRPC))

		} else {
			perRPC, err := ctrlClient.OpenRPCCredentials(ctx)
			if err != nil {
				return nil, err
			}
			opts = append(opts, grpc.WithPerRPCCredentials(perRPC))
		}
	}

	creds, err := loadTransportCredentials(ctx)
	if err != nil {
		return nil, err
	}
	opts = append(opts, grpc.WithTransportCredentials(creds))

	cc, err := grpc.DialContext(ctx, addr, opts...)
	if err != nil {
		return nil, err
	}
	return cc, nil
}

func run(ctx context.Context, opts *Options) (err error) {
	if err := loop(ctx, opts); err != io.EOF {
		return err
	}
	os.Stdout.WriteString("\n") // break EOF
	return err
}

func exec(ctx context.Context, opts *Options) (err error) {
	src := opts.Source
	if client := opts.Remote; client != nil {
		stream, err := client.RunOnThread(ctx)
		if err != nil {
			return err
		}

		cmd := &workerpb.Command{
			Name: "default", // TODO: name?
			Exec: &workerpb.Command_Input{
				Input: src,
			},
		}
		if err := stream.Send(cmd); err != nil {
			return err
		}

		res, err := stream.Recv()
		if err != nil {
			return err
		}
		if output := res.GetOutput(); output != nil {
			if output.Output != "" {
				fmt.Println(output.Output)
			}
			if output.Status != nil {
				starlib.FprintStatus(os.Stderr, output.Status)
			}
		}
		return nil
	}

	globals := starlib.NewGlobals()
	loader := starlib.NewLoader(globals)
	defer loader.Close()

	thread := &starlark.Thread{
		Name:  opts.Filename,
		Load:  loader.Load,
		Print: printer(),
	}
	starlarkthread.SetContext(thread, ctx)
	close := starlarkthread.WithResourceStore(thread)
	defer func() {
		cerr := close()
		if err == nil {
			err = cerr
		}
	}()

	module, err := starlark.ExecFile(thread, opts.Filename, src, globals)
	if err != nil {
		return err
	}
	mainFn, ok := module["main"]
	if !ok {
		return nil
	}
	if _, err := starlark.Call(thread, mainFn, nil, nil); err != nil {
		return err
	}
	return nil
}

func start(ctx context.Context, filename, src string) error {
	var dir string
	if name := *flagCacheDir; name != "" {
		if f, err := os.Stat(name); err != nil {
			return fmt.Errorf("error: invalid cache dir: %w", err)
		} else if !f.IsDir() {
			return fmt.Errorf("error: invalid cache dir: %s", name)
		}
		dir = name
	}

	if dir == "" {
		dirname, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dir = filepath.Join(dirname, ".cache", "larking")
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			return err
		}
	}

	ctrl, err := control.NewClient(*flagControlAddr, dir)
	if err != nil {
		return err
	}

	var client workerpb.WorkerClient
	if remoteAddr := *flagRemoteAddr; remoteAddr != "" {
		cc, err := createRemoteConn(ctx, remoteAddr, ctrl)
		if err != nil {
			return err
		}
		defer cc.Close()

		log.Printf("remote: %s, status: %s", cc.Target(), cc.GetState())

		client = workerpb.NewWorkerClient(cc)
	}

	var historyFile string
	if dir != "" {
		historyFile = filepath.Join(dir, "history.txt")
	}
	autocomplete := *flagAutocomplete

	opts := &Options{
		CacheDir:     dir,
		HistoryFile:  historyFile,
		AutoComplete: autocomplete,
		Remote:       client,
		Filename:     filename,
		Source:       src,
	}

	if opts.Source != "" { // TODO: flag better?
		return exec(ctx, opts)
	}
	return run(ctx, opts)
}

func test(ctx context.Context, pattern string) error {
	if _, err := path.Match(pattern, ""); err != nil {
		return err // invalid pattern
	}

	globals := starlib.NewGlobals()
	loader := starlib.NewLoader(globals)
	defer loader.Close()

	runner := func(t testing.TB, thread *starlark.Thread) func() {
		thread.Load = loader.Load

		starlarkthread.SetContext(thread, ctx)

		close := starlarkthread.WithResourceStore(thread)
		return func() {
			if err := close(); err != nil {
				t.Error(err)
			}
		}
	}

	bkt, err := blob.OpenBucket(ctx, *flagDir)
	if err != nil {
		return err
	}
	defer bkt.Close()

	// Limit choice by prefix, path.Match the rest.
	opts := &blob.ListOptions{
		Prefix: pattern,
	}
	if i := strings.IndexAny(pattern, "*?[\\"); i >= 0 {
		opts.Prefix = pattern[:i]
	}

	var tests []testing.InternalTest
	iter := bkt.List(opts)
	for {
		obj, err := iter.Next(ctx)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if ok, _ := path.Match(pattern, obj.Key); !ok {
			continue
		}

		src, err := bkt.ReadAll(ctx, obj.Key)
		if err != nil {
			return err
		}

		tests = append(tests, testing.InternalTest{
			Name: obj.Key,
			F: func(t *testing.T) {
				starlarkassert.TestFile(
					t, obj.Key, src, globals, runner)
			},
		})
	}

	var (
		matchPat string
		matchRe  *regexp.Regexp
	)
	deps := starlarkassert.MatchStringOnly(
		func(pat, str string) (result bool, err error) {
			if matchRe == nil || matchPat != pat {
				matchPat = pat
				matchRe, err = regexp.Compile(matchPat)
				if err != nil {
					return
				}
			}
			return matchRe.MatchString(str), nil
		},
	)
	if testing.MainStart(deps, tests, nil, nil, nil).Run() > 0 {
		return fmt.Errorf("failed")
	}

	return nil
}

func format(ctx context.Context, pattern string) error {
	if _, err := path.Match(pattern, ""); err != nil {
		return err // invalid pattern
	}

	bkt, err := blob.OpenBucket(ctx, *flagDir)
	if err != nil {
		return err
	}
	defer bkt.Close()

	// Limit choice by prefix, path.Match the rest.
	opts := &blob.ListOptions{
		Prefix: pattern,
	}
	if i := strings.IndexAny(pattern, "*?[\\"); i >= 0 {
		opts.Prefix = pattern[:i]
	}

	iter := bkt.List(opts)
	for {
		obj, err := iter.Next(ctx)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if ok, _ := path.Match(pattern, obj.Key); !ok {
			continue
		}

		src, err := bkt.ReadAll(ctx, obj.Key)
		if err != nil {
			return err
		}

		dst, err := worker.Format(ctx, obj.Key, src)
		if err != nil {
			return err
		}

		if bytes.Equal(src, dst) {
			continue
		}

		log.Println("formatting", obj.Key)
		if err := bkt.WriteAll(ctx, obj.Key, dst, nil); err != nil {
			return err
		}
	}

	return nil

}

func main() {
	ctx := context.Background()
	log.SetPrefix("")
	log.SetFlags(0)
	flag.Parse()

	var arg0 string
	if flag.NArg() >= 1 {
		arg0 = flag.Arg(0)
	}

	const fileExt = ".star"

	switch {
	case arg0 == "fmt":
		pattern := "*" + fileExt
		if flag.NArg() == 2 {
			pattern = flag.Arg(1)
		}
		if err := format(ctx, pattern); err != nil {
			if err != io.EOF {
				starlib.FprintErr(os.Stderr, err)
			}
			os.Exit(1)
		}

	case arg0 == "test":
		pattern := "*_test" + fileExt
		if flag.NArg() == 2 {
			pattern = flag.Arg(1)
		}
		if err := test(ctx, pattern); err != nil {
			if err != io.EOF {
				starlib.FprintErr(os.Stderr, err)
			}
			os.Exit(1)
		}

	case flag.NArg() == 1 || *flagExecprog != "":
		var (
			filename string
			src      string
		)
		if *flagExecprog != "" {
			// Execute provided program.
			filename = "cmdline"
			src = *flagExecprog
		} else {
			// Execute specified file.
			filename = arg0

			var err error
			b, err := ioutil.ReadFile(filename)
			if err != nil {
				log.Fatal(err)
			}
			src = string(b)
		}
		if err := start(ctx, filename, src); err != nil {
			starlib.FprintErr(os.Stderr, err)
			os.Exit(1)
		}
	case flag.NArg() == 0:
		text := `   _,
  ( '>   Welcome to lark
 / ) )   (larking.io, %s)
 /|^^
`
		fmt.Printf(text, runtime.Version())
		if err := start(ctx, "<stdin>", ""); err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatal("want at most one Starlark file name")
	}

}
