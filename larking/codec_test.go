package larking

import (
	"bytes"
	"errors"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"larking.io/api/testpb"
)

func TestStreamCodecs(t *testing.T) {

	protob, err := proto.Marshal(&testpb.Message{
		Text: "hello, protobuf",
	})
	if err != nil {
		t.Fatal(err)
	}
	protobig, err := proto.Marshal(&testpb.Message{
		Text: string(bytes.Repeat([]byte{'a'}, 1<<20)),
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
	jsonescape := []byte(`{"text":"hello, json} \" }}"}`)

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
			b := protowire.AppendVarint(nil, uint64(len(protob)))
			t.Log("len(b)=", len(b))
			return append(b, protob...)
		}(),
		want: protob,
	}, {
		name:  "proto unbuffered",
		codec: CodecProto{},
		input: make([]byte, 0, 4+len(protob)),
		extra: func() []byte {
			b := protowire.AppendVarint(nil, uint64(len(protob)))
			return append(b, protob...)
		}(),
		want: protob,
	}, {
		name:  "proto partial size",
		codec: CodecProto{},
		input: func() []byte {
			b := protowire.AppendVarint(nil, uint64(len(protob)))
			return append(b, protob...)[:1]
		}(),
		extra: func() []byte {
			b := protowire.AppendVarint(nil, uint64(len(protob)))
			return append(b, protob...)[1:]
		}(),
		want: protob,
	}, {
		name:  "proto partial message",
		codec: CodecProto{},
		input: func() []byte {
			b := protowire.AppendVarint(nil, uint64(len(protob)))
			return append(b, protob...)[:6]
		}(),
		extra: func() []byte {
			b := protowire.AppendVarint(nil, uint64(len(protob)))
			return append(b, protob...)[6:]
		}(),
		want: protob,
	}, {
		name:  "proto zero size",
		codec: CodecProto{},
		extra: func() []byte {
			b := protowire.AppendVarint(nil, 0)
			return b
		}(),
		want: []byte{},
	}, {
		name:  "proto big size",
		codec: CodecProto{},
		extra: func() []byte {
			b := protowire.AppendVarint(nil, uint64(len(protobig)))
			return append(b, protobig...)
		}(),
		want: protobig,
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
	}, {
		name:  "json escape",
		codec: CodecJSON{},
		input: jsonescape,
		want:  jsonescape,
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			codec := tt.codec.(StreamCodec)

			r := bytes.NewReader(tt.extra)
			b, n, err := codec.ReadNext(tt.input, r, len(tt.want))
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
				t.Errorf("got %s, want %s", got, tt.want)
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
			if _, err := codec.WriteNext(&buf, b); err != nil {
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
