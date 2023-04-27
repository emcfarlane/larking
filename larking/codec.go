// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package larking

import (
	"fmt"
	"sync"

	"google.golang.org/grpc/encoding"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

var bytesPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 0, 64)
		return &b
	},
}

// Codec defines the interface used to encode and decode messages.
type Codec interface {
	encoding.Codec
	// MarshalAppend appends the marshaled form of v to b and returns the result.
	MarshalAppend([]byte, interface{}) ([]byte, error)
}

func init() {
	encoding.RegisterCodec(CodecProto{})
	encoding.RegisterCodec(CodecJSON{})
}

func errInvalidType(v any) error {
	return fmt.Errorf("marshal invalid type %T", v)
}

// CodecProto is a Codec implementation with protobuf binary format.
type CodecProto struct {
	proto.MarshalOptions
}

func (c CodecProto) Marshal(v interface{}) ([]byte, error) {
	m, ok := v.(proto.Message)
	if !ok {
		return nil, errInvalidType(v)
	}
	return c.MarshalOptions.Marshal(m)
}

func (c CodecProto) MarshalAppend(b []byte, v interface{}) ([]byte, error) {
	m, ok := v.(proto.Message)
	if !ok {
		return nil, errInvalidType(v)
	}
	return c.MarshalOptions.MarshalAppend(b, m)
}

func (CodecProto) Unmarshal(data []byte, v interface{}) error {
	m, ok := v.(proto.Message)
	if !ok {
		return errInvalidType(v)
	}
	return proto.Unmarshal(data, m)
}

// Name == "proto" overwritting internal proto codec
func (CodecProto) Name() string { return "proto" }

// CodecJSON is a Codec implementation with protobuf json format.
type CodecJSON struct {
	protojson.MarshalOptions
}

func (c CodecJSON) Marshal(v interface{}) ([]byte, error) {
	m, ok := v.(proto.Message)
	if !ok {
		return nil, errInvalidType(v)
	}
	return c.MarshalOptions.Marshal(m)
}

func (c CodecJSON) MarshalAppend(b []byte, v interface{}) ([]byte, error) {
	m, ok := v.(proto.Message)
	if !ok {
		return nil, errInvalidType(v)
	}
	// TODO: implement MarshalAppend
	return c.MarshalOptions.Marshal(m)
}

func (CodecJSON) Unmarshal(data []byte, v interface{}) error {
	m, ok := v.(proto.Message)
	if !ok {
		return errInvalidType(v)
	}
	return protojson.Unmarshal(data, m)
}

func (CodecJSON) Name() string { return "json" }
