// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlib

import (
	"fmt"
	"io"

	"go.starlark.net/starlark"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	spb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/status"
)

type statusError interface{ GRPCStatus() *status.Status }

func FprintStatus(w io.Writer, status *spb.Status) {
	for _, detail := range status.Details {
		m, err := detail.UnmarshalNew()
		if err != nil {
			fmt.Fprintf(w, "InternalError: %v\n", err)
			continue
		}
		switch m := m.(type) {
		case *errdetails.DebugInfo:
			for _, se := range m.StackEntries {
				fmt.Fprintln(w, se)
			}
		default:
			fmt.Fprintf(w, "%v\n", m)
		}
	}
	if len(status.Details) == 0 {
		fmt.Fprintf(w, "Error: %v: %s\n", status.Code, status.Message)
	}
}

func FprintErr(w io.Writer, err error) {
	switch v := err.(type) {
	case *starlark.EvalError:
		fmt.Fprintln(w, v.Backtrace())
	case statusError:
		s := v.GRPCStatus()
		p := s.Proto()
		FprintStatus(w, p)
	default:
		fmt.Fprintln(w, err)
	}
}
