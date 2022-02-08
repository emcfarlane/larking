// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package worker

import (
	"fmt"
	"strings"

	"go.starlark.net/starlark"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// errorStatus creates a status from an error,
// or its backtrace if it is a Starlark evaluation error.
func errorStatus(err error) *status.Status {
	if err == nil {
		return nil
	}
	st, ok := status.FromError(err)
	if !ok {
		st = status.New(codes.InvalidArgument, err.Error())
	}
	if evalErr, ok := err.(*starlark.EvalError); ok {
		// Describes additional debugging info.
		di := &errdetails.DebugInfo{
			StackEntries: strings.Split(evalErr.Backtrace(), "\n"),
			Detail:       "<repl>",
		}
		st, err = st.WithDetails(di)
		if err != nil {
			// If this errored, it will always error
			// here, so better panic so we can figure
			// out why than have this silently passing.
			panic(fmt.Sprintf("Unexpected error: %v", err))
		}
	}
	return st

}
