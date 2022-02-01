// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package worker

import (
	"bytes"
	"strings"

	"github.com/go-logr/logr"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/emcfarlane/larking/api/workerpb"
	"github.com/emcfarlane/larking/starlarkthread"
	"github.com/emcfarlane/larking/starlib"
)

type Server struct {
	workerpb.UnimplementedWorkerServer
	load func(thread *starlark.Thread, module string) (starlark.StringDict, error)
}

func NewServer(
	load func(thread *starlark.Thread, module string) (starlark.StringDict, error),
) *Server {
	return &Server{
		load: load,
	}
}

func soleExpr(f *syntax.File) syntax.Expr {
	if len(f.Stmts) == 1 {
		if stmt, ok := f.Stmts[0].(*syntax.ExprStmt); ok {
			return stmt.X
		}
	}
	return nil
}

// Create ServerStream...
func (s *Server) RunOnThread(stream workerpb.Worker_RunOnThreadServer) (err error) {
	ctx := stream.Context()
	l := logr.FromContextOrDiscard(ctx)

	cmd, err := stream.Recv()
	if err != nil {
		return err
	}
	l.Info("running on thread", "thread", cmd.Name)

	var buf bytes.Buffer
	thread := &starlark.Thread{
		Name: "<worker>",
		Print: func(_ *starlark.Thread, msg string) {
			buf.WriteString(msg) //nolint
		},
		Load: s.load,
	}

	starlarkthread.SetContext(thread, ctx)
	cleanup := starlarkthread.WithResourceStore(thread)
	defer func() {
		if cerr := cleanup(); err == nil {
			err = cerr
		}
		l.Info("thread closed", "err", err)
	}()

	module := strings.TrimPrefix(cmd.Name, "thread/")

	globals := starlib.NewGlobals()
	if module != "" {
		if s.load == nil {
			return status.Error(codes.Unavailable, "module loading not avaliable")
		}
		predeclared, err := s.load(thread, module)
		if err != nil {
			return err
		}
		for key, val := range predeclared {
			globals[key] = val // copy thread values to globals
		}
		thread.Name = "<worker:" + module + ">"
	}

	run := func(input string) error {
		buf.Reset()
		f, err := syntax.Parse(thread.Name, input, 0)
		if err != nil {
			return err
		}

		if expr := soleExpr(f); expr != nil {
			// eval
			v, err := starlark.EvalExpr(thread, expr, globals)
			if err != nil {
				return err
			}

			// print
			if v != starlark.None {
				buf.WriteString(v.String())
			}
		} else if err := starlark.ExecREPLChunk(f, thread, globals); err != nil {
			return err
		}
		return nil
	}

	c := starlib.Completer{StringDict: globals}
	for {
		result := &workerpb.Result{}

		switch v := cmd.Exec.(type) {
		case *workerpb.Command_Input:
			err := run(v.Input)
			if err != nil {
				l.Info("thread error", "err", err)
			}
			result.Result = &workerpb.Result_Output{
				Output: &workerpb.Output{
					Output: buf.String(),
					Status: errorStatus(err).Proto(),
				},
			}

		case *workerpb.Command_Complete:
			completions := c.Complete(v.Complete)
			result.Result = &workerpb.Result_Completion{
				Completion: &workerpb.Completion{
					Completions: completions,
				},
			}

		}
		if err = stream.Send(result); err != nil {
			return err
		}

		cmd, err = stream.Recv()
		if err != nil {
			return err
		}
	}
}
