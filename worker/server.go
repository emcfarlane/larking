// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package worker

import (
	"bytes"
	"fmt"

	"github.com/go-logr/logr"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/emcfarlane/larking/api/workerpb"
	"github.com/emcfarlane/larking/starlarkthread"
	"github.com/emcfarlane/larking/starlib"
)

type Server struct {
	workerpb.UnimplementedWorkerServer

	Load func(thread *starlark.Thread, module string) (starlark.StringDict, error)
}

func NewServer() *Server {
	return &Server{}
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
	fmt.Println("RUN ON THREAD")
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
		Load: s.Load,
	}

	starlarkthread.SetContext(thread, ctx)
	cleanup := starlarkthread.WithResourceStore(thread)
	defer func() {
		if cerr := cleanup(); err == nil {
			err = cerr
		}
	}()

	var globals starlark.StringDict
	if cmd.Name != "" {
		globals, err = s.Load(thread, cmd.Name)
		if err != nil {
			return err
		}
		thread.Name = "<worker:" + cmd.Name + ">"
	}

	c := starlib.Completer{StringDict: globals}

	for {
		switch v := cmd.Exec.(type) {
		case *workerpb.Command_Input:
			buf.Reset()

			f, err := syntax.Parse(thread.Name, v.Input, 0)
			//f, err := syntax.ParseCompoundStmt(fmt.Sprintf("<%s>", serverThread.name), readline)
			if err != nil {
				return errorStatus(err).Err()
			}

			if expr := soleExpr(f); expr != nil {
				// eval
				v, err := starlark.EvalExpr(thread, expr, globals)
				if err != nil {
					return errorStatus(err).Err()
				}

				// print
				if v != starlark.None {
					buf.WriteString(v.String())
				}
			} else if err := starlark.ExecREPLChunk(f, thread, globals); err != nil {
				return errorStatus(err).Err()
			}

			result := &workerpb.Result{
				Result: &workerpb.Result_Output{
					Output: &workerpb.Output{
						//Input:  v.Input,
						Output: buf.String(),
					},
				},
			}

			if err = stream.Send(result); err != nil {
				return err
			}

		case *workerpb.Command_Complete:
			completions := c.Complete(v.Complete)

			result := &workerpb.Result{
				Result: &workerpb.Result_Completion{
					Completion: &workerpb.Completion{
						Completions: completions,
					},
				},
			}

			if err = stream.Send(result); err != nil {
				return err
			}
		}

		cmd, err = stream.Recv()
		if err != nil {
			return err
		}
	}
}
