package larking

import (
	"bytes"
	"compress/gzip"
	"io"
	"sync"

	"google.golang.org/grpc/encoding"
)

var bufPool = sync.Pool{
	New: func() interface{} {
		return &bytes.Buffer{}
	},
}

// Compressor is used to compress and decompress messages.
// Based on grpc/encoding.
type Compressor interface {
	encoding.Compressor
}

// CompressorGzip implements the Compressor interface.
// Based on grpc/encoding/gzip.
type CompressorGzip struct {
	Level            *int
	poolCompressor   sync.Pool
	poolDecompressor sync.Pool
}

// Name returns gzip.
func (*CompressorGzip) Name() string { return "gzip" }

type gzipWriter struct {
	*gzip.Writer
	pool *sync.Pool
}

// Compress implements the Compressor interface.
func (c *CompressorGzip) Compress(w io.Writer) (io.WriteCloser, error) {
	z, ok := c.poolCompressor.Get().(*gzipWriter)
	if !ok {
		level := gzip.DefaultCompression
		if c.Level != nil {
			level = *c.Level
		}
		newZ, err := gzip.NewWriterLevel(w, level)
		if err != nil {
			return nil, err
		}
		return &gzipWriter{Writer: newZ, pool: &c.poolCompressor}, nil
	}
	z.Reset(w)
	return z, nil
}

func (z *gzipWriter) Close() error {
	defer z.pool.Put(z)
	return z.Writer.Close()
}

type gzipReader struct {
	*gzip.Reader
	pool *sync.Pool
}

// Decompress implements the Compressor interface.
func (c *CompressorGzip) Decompress(r io.Reader) (io.Reader, error) {
	z, ok := c.poolDecompressor.Get().(*gzipReader)
	if !ok {
		newZ, err := gzip.NewReader(r)
		if err != nil {
			return nil, err
		}
		return &gzipReader{Reader: newZ, pool: &c.poolDecompressor}, nil
	}
	if err := z.Reset(r); err != nil {
		z.pool.Put(z)
		return nil, err
	}
	return z, nil
}

func (z *gzipReader) Read(p []byte) (n int, err error) {
	n, err = z.Reader.Read(p)
	if err == io.EOF {
		z.pool.Put(z)
	}
	return n, err
}
