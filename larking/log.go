// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package larking

import (
	"context"

	"google.golang.org/grpc"
)

type logStream struct {
	grpc.ServerStream
	info  *grpc.StreamServerInfo
	ctxFn NewContextFunc
}

func (s logStream) Context() context.Context {
	return s.ctxFn(s.ServerStream.Context(), s.info.FullMethod, s.info.IsClientStream, s.info.IsServerStream)
}

// NewContextFunc is a function that creates a new context for a request.
// The returned context is used for the duration of the request.
type NewContextFunc func(ctx context.Context, fullMethod string, isClientStream, isServerStream bool) context.Context

// NewUnaryContext returns a UnaryServerInterceptor that calls ctxFn to
// create a new context for each request.
func NewUnaryContext(ctxFn NewContextFunc) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		ctx = ctxFn(ctx, info.FullMethod, false, false)
		return handler(ctx, req)
	}
}

// NewStreamContext returns a StreamServerInterceptor that calls ctxFn to
// create a new context for each request.
func NewStreamContext(ctxFn NewContextFunc) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		return handler(srv, logStream{
			ServerStream: ss,
			info:         info,
			ctxFn:        ctxFn,
		})
	}
}
