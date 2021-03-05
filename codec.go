package larking

import (
	"fmt"

	"google.golang.org/grpc/encoding"
	"google.golang.org/protobuf/proto"
)

func init() {
	encoding.RegisterCodec(codec{})
}

type codec struct{}

func (codec) Marshal(v interface{}) ([]byte, error) {
	m, ok := v.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("marshal invalid type %T", v)
	}
	return proto.Marshal(m)
}

func (codec) Unmarshal(data []byte, v interface{}) error {
	m, ok := v.(proto.Message)
	if !ok {
		return fmt.Errorf("unmarshal invalid type %T", v)
	}
	return proto.Unmarshal(data, m)
}

// Name == "proto" overwritting internal proto codec
func (codec) Name() string { return "proto" }
