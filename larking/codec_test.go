package larking

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"
	"larking.io/api/testpb"
)

func TestSizeCodecs(t *testing.T) {

	protob, err := proto.Marshal(&testpb.Message{
		Text: "hello, protobuf",
	})
	if err != nil {
		t.Fatal(err)
	}
	jsonb, err := (&CodecJSON{}).Marshal(&testpb.Message{
		Text: "hello, json",
	})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		codec   Codec
		input   []byte
		extra   []byte
		want    []byte
		wantErr error
	}{{
		name:  "proto buffered",
		codec: CodecProto{},
		input: func() []byte {
			var buf [4]byte
			binary.BigEndian.PutUint32(buf[:], uint32(len(protob)))
			return append(buf[:], protob...)
		}(),
		want: protob,
	}, {
		name:  "proto unbuffered",
		codec: CodecProto{},
		input: make([]byte, 0, 4+len(protob)),
		extra: func() []byte {
			var buf [4]byte
			binary.BigEndian.PutUint32(buf[:], uint32(len(protob)))
			return append(buf[:], protob...)
		}(),
		want: protob,
	}, {
		name:  "proto partial size",
		codec: CodecProto{},
		input: func() []byte {
			var buf [4]byte
			binary.BigEndian.PutUint32(buf[:], uint32(len(protob)))
			return append(buf[:], protob...)[:2]
		}(),
		extra: func() []byte {
			var buf [4]byte
			binary.BigEndian.PutUint32(buf[:], uint32(len(protob)))
			return append(buf[:], protob...)[2:]
		}(),
		want: protob,
	}, {
		name:  "proto partial message",
		codec: CodecProto{},
		input: func() []byte {
			var buf [4]byte
			binary.BigEndian.PutUint32(buf[:], uint32(len(protob)))
			return append(buf[:], protob...)[:6]
		}(),
		extra: func() []byte {
			var buf [4]byte
			binary.BigEndian.PutUint32(buf[:], uint32(len(protob)))
			return append(buf[:], protob...)[6:]
		}(),
		want: protob,
	}, {
		name:  "json buffered",
		codec: CodecJSON{},
		input: jsonb,
		want:  jsonb,
	}, {
		name:  "json unbuffered",
		codec: CodecJSON{},
		input: make([]byte, 0, 4+len(jsonb)),
		extra: jsonb,
		want:  jsonb,
	}, {
		name:  "json partial object",
		codec: CodecJSON{},
		input: jsonb[:2],
		extra: jsonb[2:],
		want:  jsonb,
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			codec := tt.codec.(SizeCodec)

			r := bytes.NewReader(tt.extra)
			b, n, err := codec.SizeRead(tt.input, r, len(tt.want))
			if err != nil {
				if tt.wantErr != nil {
					if !errors.Is(err, tt.wantErr) {
						t.Fatalf("got %v, want %v", err, tt.wantErr)
					}
					return
				}
				t.Fatal(err)
			}
			if n > len(b) {
				t.Fatalf("n %v > %v", n, len(b))
			}

			got := b[:n]
			if !bytes.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}

			var msg testpb.Message
			if err := codec.Unmarshal(got, &msg); err != nil {
				t.Error(err)
			}

			b, err = codec.MarshalAppend(b[:0], &msg)
			if err != nil {
				t.Fatal(err)
			}

			var buf bytes.Buffer
			if _, err := codec.SizeWrite(&buf, b); err != nil {
				t.Fatal(err)
			}

			onwire := append([]byte(nil), tt.input...)
			onwire = append(onwire, tt.extra...)
			if !bytes.Equal(buf.Bytes(), onwire) {
				t.Errorf("onwire got %v, want %v", buf.Bytes(), onwire)
			}
		})
	}
}
