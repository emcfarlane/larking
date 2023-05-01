// Copyright 2023 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package larking

import (
	"encoding/binary"
	"fmt"
	"io"
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

// SizeCodec is used in streaming RPCs where the message boundaries are
// determined by the codec.
type SizeCodec interface {
	Codec

	// SizeRead returns the size of the next message appended to dst.
	// SizeRead reads from r until either it has read a complete message or
	// encountered an error. SizeRead returns the data read from r.
	// The message is contained in dst[:n].
	// Excess data read from r is stored in dst[n:].
	SizeRead(dst []byte, r io.Reader, limit int) ([]byte, int, error)
	// SizeWrite writes the message to w with a size aware encoding
	// returning the number of bytes written.
	SizeWrite(w io.Writer, src []byte) (int, error)
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

// SizeRead reads the length of the message encoded as 4 byte unsigned integer
// and then reads the message from r.
func (c CodecProto) SizeRead(b []byte, r io.Reader, limit int) ([]byte, int, error) {
	var buf [4]byte
	copy(buf[:], b)
	if len(b) < 4 {
		if _, err := r.Read(buf[len(b):]); err != nil {
			return b, 0, err
		}
		b = b[len(b):] // truncate
	} else {
		b = b[4:] // truncate
	}

	// Size of the message is encoded as 4 byte unsigned integer.
	u := binary.BigEndian.Uint32(buf[:])
	if int(u) > limit {
		return b, 0, fmt.Errorf("message size %d exceeds limit %d", u, limit)
	}

	if len(b) < int(u) {
		if cap(b) < int(u) {
			dst := make([]byte, len(b), int(u))
			copy(dst, b)
			b = dst
		}
		if _, err := r.Read(b[len(b):int(u)]); err != nil {
			return b, 0, err
		}
		b = b[:u]
	}
	return b, int(u), nil
}

// SizeWrite writes the length of the message encoded as 4 byte unsigned integer
// and then writes the message to w.
func (c CodecProto) SizeWrite(w io.Writer, b []byte) (int, error) {
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], uint32(len(b)))
	if _, err := w.Write(buf[:]); err != nil {
		return 0, err
	}
	return w.Write(b)
}

// Name == "proto" overwritting internal proto codec
func (CodecProto) Name() string { return "proto" }

// CodecJSON is a Codec implementation with protobuf json format.
type CodecJSON struct {
	protojson.MarshalOptions
	protojson.UnmarshalOptions
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
	return c.MarshalOptions.MarshalAppend(b, m)
}

func (c CodecJSON) Unmarshal(data []byte, v interface{}) error {
	m, ok := v.(proto.Message)
	if !ok {
		return errInvalidType(v)
	}
	return c.UnmarshalOptions.Unmarshal(data, m)
}

// SizeRead reads the length of the message around the json object.
// It reads until it finds a matching number of braces.
// It does not validate the JSON.
func (c CodecJSON) SizeRead(b []byte, r io.Reader, limit int) ([]byte, int, error) {
	var (
		braceCount int
		isString   bool
		isEscaped  bool
	)
	for i := 0; i < int(limit); i++ {
		for i >= len(b) {
			if len(b) == cap(b) {
				// Add more capacity (let append pick how much).
				b = append(b, 0)[:len(b)]
			}
			n, err := r.Read(b[len(b):cap(b)])
			b = b[:len(b)+n]
			if err != nil {
				return b, 0, err
			}
		}

		switch {
		case isString:
			switch b[i] {
			case '\\':
				isEscaped = true
			case '"':
				isString = false
			}
		case isEscaped:
			isEscaped = false
		default:
			switch b[i] {
			case '{':
				braceCount++
			case '}':
				braceCount--
				if braceCount == 0 {
					return b, i + 1, nil
				}
				if braceCount < 0 {
					return b, 0, fmt.Errorf("unbalanced braces")
				}
			case '"':
				isString = true
			}
		}
	}
	return b, 0, fmt.Errorf("message greater than limit %d", limit)
}

// SizeWrite writes the raw JSON message to w without any size prefix.
func (c CodecJSON) SizeWrite(w io.Writer, b []byte) (int, error) {
	return w.Write(b)
}

func (CodecJSON) Name() string { return "json" }
