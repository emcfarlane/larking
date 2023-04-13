// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package larking

import (
	"context"

	"github.com/go-logr/logr"
	"google.golang.org/grpc"
)

type logStream struct {
	grpc.ServerStream
	log logr.Logger
}

func (s logStream) Context() context.Context {
	return logr.NewContext(s.ServerStream.Context(), s.log)
}

func NewUnaryContextLogr(log logr.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		ctx = logr.NewContext(ctx, log)
		return handler(ctx, req)
	}
}

func NewStreamContextLogr(log logr.Logger) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		return handler(srv, logStream{
			ServerStream: ss,
			log:          log,
		})
	}
}
