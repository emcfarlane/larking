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
	"google.golang.org/protobuf/encoding/protodelim"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

var bytesPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 0, 64)
		return &b
	},
}

// growcap scales up the capacity of a slice.
// Taken from the Go 1.14 runtime and proto package.
func growcap(oldcap, wantcap int) (newcap int) {
	if wantcap > oldcap*2 {
		newcap = wantcap
	} else if oldcap < 1024 {
		// The Go 1.14 runtime takes this case when len(s) < 1024,
		// not when cap(s) < 1024. The difference doesn't seem
		// significant here.
		newcap = oldcap * 2
	} else {
		newcap = oldcap
		for 0 < newcap && newcap < wantcap {
			newcap += newcap / 4
		}
		if newcap <= 0 {
			newcap = wantcap
		}
	}
	return newcap
}

// Codec defines the interface used to encode and decode messages.
type Codec interface {
	encoding.Codec
	// MarshalAppend appends the marshaled form of v to b and returns the result.
	MarshalAppend([]byte, interface{}) ([]byte, error)
}

// StreamCodec is used in streaming RPCs where the message boundaries are
// determined by the codec.
type StreamCodec interface {
	Codec

	// ReadNext returns the size of the next message appended to buf.
	// ReadNext reads from r until either it has read a complete message or
	// encountered an error and returns all the data read from r.
	// The message is contained in dst[:n].
	// Excess data read from r is stored in dst[n:].
	ReadNext(buf []byte, r io.Reader, limit int) (dst []byte, n int, err error)
	// WriteNext writes the message to w with a size aware encoding
	// returning the number of bytes written.
	WriteNext(w io.Writer, src []byte) (n int, err error)
}

func errInvalidType(v any) error {
	return fmt.Errorf("marshal invalid type %T", v)
}

// vtMessage implements planetscale MessageVT interface.
type vtMessage interface {
	SizeVT() int
	MarshalToSizedBufferVT([]byte) (int, error)
	UnmarshalVT([]byte) error
}

// CodecProto is a Codec implementation with protobuf binary format.
type CodecProto struct {
	proto.MarshalOptions
}

func (c CodecProto) Marshal(v interface{}) ([]byte, error) {
	switch m := v.(type) {
	case vtMessage:
		size := m.SizeVT()
		dAtA := make([]byte, size)
		n, err := m.MarshalToSizedBufferVT(dAtA[:size])
		if err != nil {
			return nil, err
		}
		return dAtA[:n], nil
	case proto.Message:
		return c.MarshalOptions.Marshal(m)
	default:
		return nil, errInvalidType(v)
	}
}

func (c CodecProto) MarshalAppend(b []byte, v interface{}) ([]byte, error) {
	switch m := v.(type) {
	case vtMessage:
		size := m.SizeVT()
		if cap(b) < size {
			dst := make([]byte, len(b), growcap(cap(b), size))
			copy(dst, b)
			b = dst
		}
		n, err := m.MarshalToSizedBufferVT(b[:size])
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	case proto.Message:
		return c.MarshalOptions.MarshalAppend(b, m)
	default:
		return nil, errInvalidType(v)
	}
}

func (CodecProto) Unmarshal(data []byte, v interface{}) error {
	switch m := v.(type) {
	case vtMessage:
		return m.UnmarshalVT(data)
	case proto.Message:
		return proto.Unmarshal(data, m)
	default:
		return errInvalidType(v)
	}
}

// ReadNext reads a varint size-delimited wire-format message from r.
func (c CodecProto) ReadNext(b []byte, r io.Reader, limit int) ([]byte, int, error) {
	for i := 0; i < binary.MaxVarintLen64; i++ {
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
		if b[i] < 0x80 {
			break
		}
	}

	size, n := protowire.ConsumeVarint(b)
	if n < 0 {
		return b, 0, protowire.ParseError(n)
	}
	if limit > 0 && int(size) > limit {
		return b, 0, &protodelim.SizeTooLargeError{Size: size, MaxSize: uint64(limit)}
	}
	b = b[n:] // consume the varint
	n = int(size)

	if len(b) < n {
		if cap(b) < n {
			dst := make([]byte, len(b), growcap(cap(b), n))
			copy(dst, b)
			b = dst
		}
		if _, err := io.ReadFull(r, b[len(b):n]); err != nil {
			if err == io.EOF {
				return b, 0, io.ErrUnexpectedEOF
			}
			return b, 0, err
		}
		b = b[:n]
	}
	return b, n, nil
}

// WriteNext writes the length of the message encoded as 4 byte unsigned integer
// and then writes the message to w.
func (c CodecProto) WriteNext(w io.Writer, b []byte) (int, error) {
	var sizeArr [binary.MaxVarintLen64]byte
	sizeBuf := protowire.AppendVarint(sizeArr[:0], uint64(len(b)))
	if _, err := w.Write(sizeBuf); err != nil {
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

// ReadNext reads the length of the message around the json object.
// It reads until it finds a matching number of braces.
// It does not validate the JSON.
func (c CodecJSON) ReadNext(b []byte, r io.Reader, limit int) ([]byte, int, error) {
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
		case isEscaped:
			isEscaped = false
		case isString:
			switch b[i] {
			case '\\':
				isEscaped = true
			case '"':
				isString = false
			}
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
	return b, 0, &protodelim.SizeTooLargeError{Size: uint64(len(b)), MaxSize: uint64(limit)}
}

// WriteNext writes the raw JSON message to w without any size prefix.
func (c CodecJSON) WriteNext(w io.Writer, b []byte) (int, error) {
	return w.Write(b)
}

func (CodecJSON) Name() string { return "json" }

type codecHTTPBody struct{}

func (codecHTTPBody) Marshal(v interface{}) ([]byte, error) {
	panic("not implemented")
}

func (codecHTTPBody) MarshalAppend(b []byte, v interface{}) ([]byte, error) {
	panic("not implemented")
}

func (codecHTTPBody) Unmarshal(data []byte, v interface{}) error {
	panic("not implemented")
}

func (codecHTTPBody) Name() string { return "body" }

func (codecHTTPBody) ReadNext(b []byte, r io.Reader, limit int) ([]byte, int, error) {
	var total int
	for {
		if len(b) == cap(b) {
			// Add more capacity (let append pick how much).
			b = append(b, 0)[:len(b)]
		}
		n, err := r.Read(b[len(b):cap(b)])
		b = b[:len(b)+n]
		total += int(n)
		if total > limit {
			total = limit
		}
		if err != nil || total == limit {
			return b, total, err
		}
	}
}

func (codecHTTPBody) WriteNext(w io.Writer, b []byte) (int, error) {
	return w.Write(b)
}
