package larking

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type streamHTTP struct {
	opts        muxOptions
	ctx         context.Context
	method      *method
	wmu         sync.Mutex
	w           io.Writer
	wCodec      Codec // nilable
	wHeader     http.Header
	rmu         sync.Mutex
	rbuf        []byte
	r           io.Reader
	rCodec      Codec // nilable
	rHeader     http.Header
	header      metadata.MD
	trailer     metadata.MD
	params      params
	accept      string
	contentType string
	recvCount   int
	sendCount   int
	sentHeader  bool
	hasBody     bool // HTTP method has a body
}

func (s *streamHTTP) SetHeader(md metadata.MD) error {
	s.wmu.Lock()
	defer s.wmu.Unlock()

	if s.sentHeader {
		return fmt.Errorf("already sent headers")
	}
	s.header = metadata.Join(s.header, md)
	return nil
}
func (s *streamHTTP) SendHeader(md metadata.MD) error {
	s.wmu.Lock()
	defer s.wmu.Unlock()

	if s.sentHeader {
		return fmt.Errorf("already sent headers")
	}
	s.header = metadata.Join(s.header, md)
	setOutgoingHeader(s.wHeader, s.header)
	// don't write the header code, wait for the body.
	s.sentHeader = true

	if sh := s.opts.statsHandler; sh != nil {
		sh.HandleRPC(s.ctx, &stats.OutHeader{
			Header:      s.header.Copy(),
			Compression: s.rHeader.Get("Accept-Encoding"),
		})
	}
	return nil
}

func (s *streamHTTP) SetTrailer(md metadata.MD) {
	s.wmu.Lock()
	defer s.wmu.Unlock()

	s.trailer = metadata.Join(s.trailer, md)
}

func (s *streamHTTP) Context() context.Context {
	sts := &serverTransportStream{s, s.method.name}
	return grpc.NewContextWithServerTransportStream(s.ctx, sts)
}

func (s *streamHTTP) writeMsg(b []byte, contentType string) (int, error) {
	s.wmu.Lock()
	defer s.wmu.Unlock()

	count := s.sendCount
	if count == 0 {
		s.wHeader.Set("Content-Type", contentType)
		setOutgoingHeader(s.wHeader, s.header, s.trailer)
	}
	s.sendCount += 1
	if s.method.desc.IsStreamingServer() {
		codec, ok := s.wCodec.(SizeCodec)
		if !ok {
			return -1, fmt.Errorf("codec %s does not support streaming", codec.Name())
		}
		if _, err := codec.SizeWrite(s.w, b); err != nil {
			return -1, err
		}
	}
	return count, s.opts.writeAll(s.w, b)
}

func (s *streamHTTP) SendMsg(m interface{}) error {
	reply := m.(proto.Message)

	if fRsp, ok := s.w.(http.Flusher); ok {
		defer fRsp.Flush()
	}

	cur := reply.ProtoReflect()
	for _, fd := range s.method.resp {
		cur = cur.Mutable(fd).Message()
	}
	msg := cur.Interface()

	if cur.Descriptor().FullName() == "google.api.HttpBody" {
		fds := cur.Descriptor().Fields()
		fdContentType := fds.ByName(protoreflect.Name("content_type"))
		fdData := fds.ByName(protoreflect.Name("data"))
		pContentType := cur.Get(fdContentType)
		pData := cur.Get(fdData)

		b := pData.Bytes()
		if _, err := s.writeMsg(b, pContentType.String()); err != nil {
			return err
		}
		if stats := s.opts.statsHandler; stats != nil {
			// TODO: raw payload stats.
			stats.HandleRPC(s.ctx, outPayload(false, m, b, b, time.Now()))
		}
		return nil
	}

	if s.wCodec == nil {
		return fmt.Errorf("unknown accept encoding: %s", s.accept)
	}

	bytes := bytesPool.Get().(*[]byte)
	b := (*bytes)[:0]
	defer func() {
		if cap(b) < s.opts.maxReceiveMessageSize {
			*bytes = b
			bytesPool.Put(bytes)
		}
	}()

	var err error
	b, err = s.wCodec.MarshalAppend(b, msg)
	if err != nil {
		return err
	}
	if _, err := s.writeMsg(b, s.contentType); err != nil {
		return err
	}
	if stats := s.opts.statsHandler; stats != nil {
		// TODO: raw payload stats.
		stats.HandleRPC(s.ctx, outPayload(false, m, b, b, time.Now()))
	}
	return nil
}

func (s *streamHTTP) readMsg(b []byte) (int, []byte, error) {
	s.rmu.Lock()
	defer s.rmu.Unlock()

	count := s.recvCount
	s.recvCount += 1
	if s.method.desc.IsStreamingClient() {
		b = append(b, s.rbuf...)
		codec, ok := s.rCodec.(SizeCodec)
		if !ok {
			return -1, nil, fmt.Errorf("codec %s does not support streaming", codec.Name())
		}
		b, n, err := codec.SizeRead(b, s.r, s.opts.maxReceiveMessageSize)
		if err != nil {
			return -1, nil, err
		}
		s.rbuf = append(s.rbuf[:0], b[n:]...)
		return count, b[:n], nil

	}
	b, err := s.opts.readAll(b, s.r)
	return count, b, err
}

func (s *streamHTTP) decodeRequestArgs(args proto.Message) (int, error) {
	bytes := bytesPool.Get().(*[]byte)
	b := (*bytes)[:0]
	defer func() {
		if cap(b) < s.opts.maxReceiveMessageSize {
			*bytes = b
			bytesPool.Put(bytes)
		}
	}()

	var (
		count int
		err   error
	)
	count, b, err = s.readMsg(b)
	if err != nil {
		return count, err
	}

	cur := args.ProtoReflect()
	for _, fd := range s.method.body {
		cur = cur.Mutable(fd).Message()
	}
	msg := cur.Interface()

	if cur.Descriptor().FullName() == "google.api.HttpBody" {
		rfl := msg.ProtoReflect()
		fds := rfl.Descriptor().Fields()
		fdContentType := fds.ByName(protoreflect.Name("content_type"))
		fdData := fds.ByName(protoreflect.Name("data"))
		rfl.Set(fdContentType, protoreflect.ValueOfString(s.contentType))
		rfl.Set(fdData, protoreflect.ValueOfBytes(b))

		if stats := s.opts.statsHandler; stats != nil {
			// TODO: raw payload stats.
			stats.HandleRPC(s.ctx, inPayload(false, msg, b, b, time.Now()))
		}
		return count, nil
	}

	if s.rCodec == nil {
		return count, fmt.Errorf("unknown content-type encoding: %s", s.contentType)
	}
	if err := s.rCodec.Unmarshal(b, msg); err != nil {
		return count, err
	}
	if stats := s.opts.statsHandler; stats != nil {
		// TODO: raw payload stats.
		stats.HandleRPC(s.ctx, inPayload(false, msg, b, b, time.Now()))
	}
	return count, nil
}

func (s *streamHTTP) RecvMsg(m interface{}) error {
	args := m.(proto.Message)

	var (
		count int
		err   error
	)
	if s.method.hasBody {
		count, err = s.decodeRequestArgs(args)
		if err != nil {
			return err
		}
	} else {
		s.rmu.Lock()
		count = s.recvCount
		s.recvCount += 1
		s.rmu.Unlock()
	}
	if count == 0 {
		if err := s.params.set(args); err != nil {
			return err
		}
	}
	return nil
}
