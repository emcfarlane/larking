// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package control

import (
	"context"

	"github.com/emcfarlane/larking/api/controlpb"
	status "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
)

type InsecureControlClient struct{}

func (InsecureControlClient) Check(
	ctx context.Context, in *controlpb.CheckRequest, opts ...grpc.CallOption,
) (*controlpb.CheckResponse, error) {
	return &controlpb.CheckResponse{
		Status: &status.Status{
			Code:    0, // okay
			Message: "insecure check",
		},
	}, nil
}
