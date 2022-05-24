package starext

import (
	"context"
	"sync"

	"gocloud.dev/blob"
)

type Blobs struct {
	mu   sync.Mutex // protects bkts
	bkts map[string]*blob.Bucket
}

func (b *Blobs) OpenBucket(ctx context.Context, urlstr string) (*blob.Bucket, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if bkt, ok := b.bkts[urlstr]; ok {
		return bkt, nil
	}

	bkt, err := blob.OpenBucket(ctx, urlstr)
	if err != nil {
		return nil, err
	}

	if b.bkts == nil {
		b.bkts = make(map[string]*blob.Bucket)
	}
	b.bkts[urlstr] = bkt
	return bkt, nil
}

// Close open buckets.
func (b *Blobs) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	var firstErr error
	for _, bkt := range b.bkts {
		if err := bkt.Close(); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (b *Blobs) NewReader(ctx context.Context, urlstr, key string, opts *blob.ReaderOptions) (*blob.Reader, error) {
	bkt, err := b.OpenBucket(ctx, urlstr)
	if err != nil {
		return nil, err
	}
	return bkt.NewReader(ctx, key, opts)
}

func (b *Blobs) NewWriter(ctx context.Context, urlstr, key string, opts *blob.WriterOptions) (*blob.Writer, error) {
	bkt, err := b.OpenBucket(ctx, urlstr)
	if err != nil {
		return nil, err
	}
	return bkt.NewWriter(ctx, key, opts)
}
