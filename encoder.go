package gateway

import (
	"encoding/binary"
	"fmt"
	"sync"
	"sync/atomic"

	"google.golang.org/grpc/encoding"
)

// Name is the name registered for the proto compressor.
const Name = "gateway"

var globalCodec = &codec{mctx: make(map[uint32]interface{})}

func init() {
	encoding.RegisterCodec(globalCodec)
}

// codec hacks around encoding context by passing lookup keys within the data.
type codec struct {
	count uint32
	mu    sync.Mutex
	mctx  map[uint32]interface{}
}

func (c *codec) encode(v interface{}) []byte {
	i := atomic.AddUint32(&c.count, 1)
	buf := make([]byte, binary.MaxVarintLen32)
	n := binary.PutUvarint(buf, uint64(i))

	c.mu.Lock()
	c.mctx[i] = v
	c.mu.Unlock()
	return buf[:n]
}

func (c *codec) decode(b []byte) (interface{}, error) {
	x, n := binary.Uvarint(b)
	if n <= 0 {
		return nil, fmt.Errorf("codec overflow")
	}
	i := uint32(x)

	c.mu.Lock()
	ctx, ok := c.mctx[i]
	delete(c.mctx, i)
	c.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("codec missing context")
	}
	return ctx, nil
}

func (c *codec) Marshal(v interface{}) ([]byte, error) {
	return c.encode(v), nil
}

func (c *codec) Unmarshal(data []byte, v interface{}) error {
	ctx, err := c.decode(data)
	if err != nil {
		return err
	}

	// TODO unmarshalling with context.
	t, ok := ctx.(*transformer)
	if !ok {
		return fmt.Errorf("codec invalid context type %T", ctx)
	}
	return t.unmarshal(v)
}

func (c *codec) Name() string { return Name }
