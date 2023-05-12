package larking

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type streamHTTP struct {
	opts           muxOptions
	ctx            context.Context
	method         *method
	w              io.Writer
	wHeader        http.Header
	rbuf           []byte    // stream read buffer
	r              io.Reader //
	rHeader        http.Header
	header         metadata.MD
	trailer        metadata.MD
	params         params
	contentType    string
	accept         string
	acceptEncoding string
	recvCount      int
	sendCount      int
	sentHeader     bool
	hasBody        bool // HTTP method has a body
	rEOF           bool // stream read EOF
}

func (s *streamHTTP) SetHeader(md metadata.MD) error {
	if s.sentHeader {
		return fmt.Errorf("already sent headers")
	}
	s.header = metadata.Join(s.header, md)
	return nil
}
func (s *streamHTTP) SendHeader(md metadata.MD) error {
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
			Compression: s.acceptEncoding,
		})
	}
	return nil
}

func (s *streamHTTP) SetTrailer(md metadata.MD) {
	s.trailer = metadata.Join(s.trailer, md)
}

func (s *streamHTTP) Context() context.Context {
	sts := &serverTransportStream{s, s.method.name}
	return grpc.NewContextWithServerTransportStream(s.ctx, sts)
}

func (s *streamHTTP) writeMsg(c Codec, b []byte, contentType string) (int, error) {
	count := s.sendCount
	if count == 0 {
		s.wHeader.Set("Content-Type", contentType)
		setOutgoingHeader(s.wHeader, s.header, s.trailer)
		s.sentHeader = true
	}
	s.sendCount += 1
	if s.method.desc.IsStreamingServer() {
		codec, ok := c.(StreamCodec)
		if !ok {
			return count, fmt.Errorf("codec %s does not support streaming", codec.Name())
		}
		_, err := codec.WriteNext(s.w, b)
		return count, err
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

	contentType := s.accept
	c, err := s.getCodec(contentType, cur)
	if err != nil {
		return err
	}

	bytes := bytesPool.Get().(*[]byte)
	b := (*bytes)[:0]
	defer func() {
		if cap(b) < s.opts.maxReceiveMessageSize {
			*bytes = b
			bytesPool.Put(bytes)
		}
	}()

	if cur.Descriptor().FullName() == "google.api.HttpBody" {
		fds := cur.Descriptor().Fields()
		fdContentType := fds.ByName(protoreflect.Name("content_type"))
		fdData := fds.ByName(protoreflect.Name("data"))
		pContentType := cur.Get(fdContentType)
		pData := cur.Get(fdData)

		b = append(b, pData.Bytes()...)
		contentType = pContentType.String()
	} else {
		var err error
		b, err = c.MarshalAppend(b, msg)
		if err != nil {
			return status.Errorf(codes.Internal, "%s: error while marshaling: %v", c.Name(), err)
		}
	}

	if _, err := s.writeMsg(c, b, contentType); err != nil {
		return err
	}
	if stats := s.opts.statsHandler; stats != nil {
		// TODO: raw payload stats.
		stats.HandleRPC(s.ctx, outPayload(false, m, b, b, time.Now()))
	}
	return nil
}

func (s *streamHTTP) readMsg(c Codec, b []byte) (int, []byte, error) {
	if s.rEOF {
		return s.recvCount, nil, io.EOF
	}

	count := s.recvCount
	s.recvCount += 1
	if s.method.desc.IsStreamingClient() {
		codec, ok := c.(StreamCodec)
		if !ok {
			return count, nil, fmt.Errorf("codec %q does not support streaming", codec.Name())
		}
		b = append(b, s.rbuf...)
		b, n, err := codec.ReadNext(b, s.r, s.opts.maxReceiveMessageSize)
		if err == io.EOF {
			s.rEOF, err = true, nil
		}
		s.rbuf = append(s.rbuf[:0], b[n:]...)
		return count, b[:n], err

	}
	b, err := s.opts.readAll(b, s.r)
	if err == io.EOF {
		s.rEOF, err = true, nil
	}
	return count, b, err
}

func (s *streamHTTP) getCodec(mediaType string, cur protoreflect.Message) (Codec, error) {
	codecType := string(cur.Descriptor().FullName())
	if c, ok := s.opts.codecs[codecType]; ok {
		return c, nil
	}
	codecType = mediaType
	if c, ok := s.opts.codecs[codecType]; ok {
		return c, nil
	}
	return nil, status.Errorf(codes.Internal, "no codec registered for content-type %q", mediaType)
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

	cur := args.ProtoReflect()
	for _, fd := range s.method.body {
		cur = cur.Mutable(fd).Message()
	}
	msg := cur.Interface()

	c, err := s.getCodec(s.contentType, cur)
	if err != nil {
		return -1, err
	}

	var (
		count int
	)
	count, b, err = s.readMsg(c, b)
	if err != nil {
		return count, err
	}

	if cur.Descriptor().FullName() == "google.api.HttpBody" {
		rfl := msg.ProtoReflect()
		fds := rfl.Descriptor().Fields()
		fdContentType := fds.ByName(protoreflect.Name("content_type"))
		fdData := fds.ByName(protoreflect.Name("data"))
		rfl.Set(fdContentType, protoreflect.ValueOfString(s.contentType))

		cpy := make([]byte, len(b))
		copy(cpy, b)
		rfl.Set(fdData, protoreflect.ValueOfBytes(cpy))
	} else {
		if err := c.Unmarshal(b, msg); err != nil {
			return count, status.Errorf(codes.Internal, "%s: error while unmarshaling: %v", c.Name(), err)
		}
	}
	if stats := s.opts.statsHandler; stats != nil {
		// TODO: raw payload stats.
		stats.HandleRPC(s.ctx, inPayload(false, msg, b, b, time.Now()))
	}
	return count, nil
}

func (s *streamHTTP) RecvMsg(m interface{}) error {
	args := m.(proto.Message)

	var count int
	if s.method.hasBody && s.hasBody {
		var err error
		count, err = s.decodeRequestArgs(args)
		if err != nil {
			return err
		}
	} else {
		count = s.recvCount
		s.recvCount += 1
		if s.rEOF {
			return io.EOF
		}
		s.rEOF = true
	}
	if count == 0 {
		if err := s.params.set(args); err != nil {
			return err
		}
	}
	return nil
}
