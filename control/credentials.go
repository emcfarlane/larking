// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package control

import (
	"bytes"
	"context"
	"fmt"
	"sync"

	"github.com/emcfarlane/larking/apipb/controlpb"
	"gocloud.dev/runtimevar"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type PerRPCCredentials struct {
	v *runtimevar.Variable

	mu    sync.Mutex
	creds *controlpb.Credentials

	err error
}

func (c *PerRPCCredentials) watch(ctx context.Context) error {
	snap, err := c.v.Watch(ctx)
	if err != nil {
		return err
	}

	var b []byte
	switch v := snap.Value.(type) {
	case string:
		b = []byte(v)
	case []byte:
		b = v
	default:
		return fmt.Errorf("unexpected PerRPCCredentials type %T", snap.Value)
	}

	var (
		creds controlpb.Credentials
	)
	if bytes.HasPrefix(b, []byte(`{`)) {
		err = protojson.Unmarshal(b, &creds)
	} else {
		err = proto.Unmarshal(b, &creds)
	}

	c.mu.Lock()
	c.creds = &creds
	c.err = err
	c.mu.Unlock()
	return err
}

func OpenRPCCredentials(ctx context.Context, u string) (*PerRPCCredentials, error) {
	v, err := runtimevar.OpenVariable(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("OpenRPCCredentials: %w", err)
	}

	c := &PerRPCCredentials{
		v: v,
	}

	if err := c.watch(ctx); err != nil {
		return nil, fmt.Errorf("OpenRPCCredentials: %w", err)
	}

	// watch
	go func() {
		ctx := context.Background()
		for {
			if err := c.watch(ctx); err == runtimevar.ErrClosed {
				break // closed
			}
		}
	}()
	return c, nil
}

func (c *PerRPCCredentials) Close() error { return c.v.Close() }

func (c *PerRPCCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.err; err != nil {
		return nil, err
	}

	switch v := c.creds.Type.(type) {
	case *controlpb.Credentials_Insecure:
		return nil, nil // nothing
	case *controlpb.Credentials_Bearer:
		return map[string]string{
			"authorization": "bearer " + v.Bearer.AccessToken,
		}, nil
	default:
		return nil, status.Errorf(codes.Unimplemented, "RPCCredentials unknown credential type")
	}
}

func (c *PerRPCCredentials) RequireTransportSecurity() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.creds.GetInsecure()
}

func (c *PerRPCCredentials) Name() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.creds.Name
}
