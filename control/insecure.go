// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package control

import (
	"context"

	"github.com/emcfarlane/larking/api/controlpb"
	"google.golang.org/grpc"
)

type InsecureControlClient struct{}

func (InsecureControlClient) Authorize(
	ctx context.Context, in *controlpb.AuthorizeRequest, opts ...grpc.CallOption,
) (*controlpb.AuthorizeResponse, error) {
	return &controlpb.AuthorizeResponse{}, nil
}
