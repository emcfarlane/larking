// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package larking

import (
	"fmt"

	"google.golang.org/grpc/encoding"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func init() {
	encoding.RegisterCodec(protoCodec{})
	encoding.RegisterCodec(jsonCodec{})
}

func errInvalidType(v any) error {
	return fmt.Errorf("marshal invalid type %T", v)
}

type protoCodec struct{}

func (protoCodec) Marshal(v interface{}) ([]byte, error) {
	m, ok := v.(proto.Message)
	if !ok {
		return nil, errInvalidType(v)
	}
	return proto.Marshal(m)
}

func (protoCodec) Unmarshal(data []byte, v interface{}) error {
	m, ok := v.(proto.Message)
	if !ok {
		return errInvalidType(v)
	}
	return proto.Unmarshal(data, m)
}

// Name == "proto" overwritting internal proto codec
func (protoCodec) Name() string { return "proto" }

type jsonCodec struct{}

func (jsonCodec) Marshal(v interface{}) ([]byte, error) {
	m, ok := v.(proto.Message)
	if !ok {
		return nil, errInvalidType(v)
	}
	return protojson.Marshal(m)
}

func (jsonCodec) Unmarshal(data []byte, v interface{}) error {
	m, ok := v.(proto.Message)
	if !ok {
		return errInvalidType(v)
	}
	return protojson.Unmarshal(data, m)
}

func (jsonCodec) Name() string { return "json" }
