package larking

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func isStreamError(err error) bool {
	switch err {
	case nil, io.EOF, context.Canceled:
		return false
	}
	return true
}

func isReservedHeader(k string) bool {
	switch k {
	case "content-type", "user-agent", "grpc-message-type", "grpc-encoding",
		"grpc-message", "grpc-status", "grpc-timeout",
		"grpc-status-details", "te":
		return true
	default:
		return false
	}
}
func isWhitelistedHeader(k string) bool {
	switch k {
	case ":authority", "user-agent":
		return true
	default:
		return false
	}
}

const binHdrSuffix = "-bin"

func encodeBinHeader(b []byte) string {
	return base64.RawStdEncoding.EncodeToString(b)
}

func decodeBinHeader(v string) (s string, err error) {
	var b []byte
	if len(v)%4 == 0 {
		// Input was padded, or padding was not necessary.
		b, err = base64.RawStdEncoding.DecodeString(v)
	} else {
		b, err = base64.RawStdEncoding.DecodeString(v)
	}
	return string(b), err
}

func newIncomingContext(ctx context.Context, header http.Header) (context.Context, metadata.MD) {
	md := make(metadata.MD, len(header))
	for k, vs := range header {
		k = strings.ToLower(k)
		if isReservedHeader(k) && !isWhitelistedHeader(k) {
			continue
		}
		if strings.HasSuffix(k, binHdrSuffix) {
			dst := make([]string, len(vs))
			for i, v := range vs {
				v, err := decodeBinHeader(v)
				if err != nil {
					continue // TODO: log error?
				}
				dst[i] = v
			}
			vs = dst
		}
		md[k] = vs
	}
	return metadata.NewIncomingContext(ctx, md), md
}

func setOutgoingHeader(header http.Header, md metadata.MD) {
	for k, vs := range md {
		if isReservedHeader(k) {
			continue
		}

		if strings.HasSuffix(k, binHdrSuffix) {
			dst := make([]string, len(vs))
			for i, v := range vs {
				dst[i] = encodeBinHeader([]byte(v))
			}
			vs = dst
		}
		header[textproto.CanonicalMIMEHeaderKey(k)] = vs
	}
}

func encodeGrpcMessage(msg string) string {
	var (
		sb  strings.Builder
		pos int
	)
	for i := 0; i < len(msg); i++ {
		c := msg[i]
		if c < ' ' || c > '~' || c == '%' {
			if pos < i {
				sb.WriteString(msg[pos:i])
			}
			sb.WriteString(fmt.Sprintf("%%%02x", c))
			pos = i + 1
		}
	}
	if pos == 0 {
		return msg
	}
	return sb.String()
}

func timeoutUnit(s byte) time.Duration {
	switch s {
	case 'H':
		return time.Hour
	case 'M':
		return time.Minute
	case 'S':
		return time.Second
	case 'm':
		return time.Millisecond
	case 'u':
		return time.Microsecond
	case 'n':
		return time.Nanosecond
	default:
		return 0
	}
}

func decodeTimeout(s string) (time.Duration, error) {
	size := len(s)
	if size < 2 {
		return 0, fmt.Errorf("transport: timeout string is too short: %q", s)
	}
	if size > 9 {
		// Spec allows for 8 digits plus the unit.
		return 0, fmt.Errorf("transport: timeout string is too long: %q", s)
	}
	d := timeoutUnit(s[size-1])
	if d == 0 {
		return 0, fmt.Errorf("transport: timeout unit is not recognized: %q", s)
	}
	t, err := strconv.ParseInt(s[:size-1], 10, 64)
	if err != nil {
		return 0, err
	}
	const maxHours = math.MaxInt64 / int64(time.Hour)
	if d == time.Hour && t > maxHours {
		// This timeout would overflow math.MaxInt64; clamp it.
		return time.Duration(math.MaxInt64), nil
	}
	return d * time.Duration(t), nil
}

type streamGRPC struct {
	opts            muxOptions
	ctx             context.Context
	done            <-chan struct{} // ctx.Done()
	wg              sync.WaitGroup
	handler         *handler
	codec           Codec      // both read and write
	comp            Compressor // both read and write
	w               io.Writer
	r               io.Reader //
	wHeader         http.Header
	header          metadata.MD
	trailer         metadata.MD
	contentType     string
	messageEncoding string
	sentHeader      bool
}

func (s *streamGRPC) isDone() error {
	select {
	case <-s.done:
		return status.FromContextError(s.ctx.Err()).Err()
	default:
		return nil
	}
}

func (s *streamGRPC) SetHeader(md metadata.MD) error {
	if s.sentHeader {
		return fmt.Errorf("already sent headers")
	}
	s.header = metadata.Join(s.header, md)
	return nil
}
func (s *streamGRPC) SendHeader(md metadata.MD) error {
	s.wg.Add(1)
	defer s.wg.Done()

	if err := s.isDone(); err != nil {
		return err
	}

	if s.sentHeader {
		return fmt.Errorf("already sent headers")
	}
	s.header = metadata.Join(s.header, md)
	h := s.wHeader

	h.Set("Content-Type", s.contentType)
	if s.messageEncoding != "" {
		h.Set("Grpc-Encoding", s.messageEncoding)
	}
	h.Add("Trailer", "Grpc-Status")
	h.Add("Trailer", "Grpc-Message")
	h.Add("Trailer", "Grpc-Status-Details-Bin")

	setOutgoingHeader(h, s.header)

	// don't write the header code, wait for the body.
	s.sentHeader = true

	if sh := s.opts.statsHandler; sh != nil {
		out := &stats.OutHeader{
			Header: s.header.Copy(),
		}
		if s.comp != nil {
			out.Compression = s.comp.Name()
		}
		sh.HandleRPC(s.ctx, out)
	}
	return nil
}

func (s *streamGRPC) SetTrailer(md metadata.MD) {
	s.trailer = metadata.Join(s.trailer, md)
}

func (s *streamGRPC) Context() context.Context {
	sts := &serverTransportStream{s, s.handler.method}
	return grpc.NewContextWithServerTransportStream(s.ctx, sts)
}
func (s *streamGRPC) compress(dst *bytes.Buffer, b []byte) error {
	w, err := s.comp.Compress(dst)
	if err != nil {
		return err
	}
	defer w.Close()
	if _, err := w.Write(b); err != nil {
		return err
	}
	return nil
}

func (s *streamGRPC) SendMsg(m interface{}) error {
	s.wg.Add(1)
	defer s.wg.Done()

	if err := s.isDone(); err != nil {
		return err
	}

	reply := m.(proto.Message)
	if !s.sentHeader {
		if err := s.SendHeader(nil); err != nil {
			return err
		}
	}

	bp := bytesPool.Get().(*[]byte)
	b := (*bp)[:0]
	defer func() {
		if cap(b) < s.opts.maxRecvMessageSize {
			*bp = b
			bytesPool.Put(bp)
		}
	}()
	if cap(b) < 5 {
		b = make([]byte, 0, growcap(cap(b), 5))
	}
	b = b[:5] // 1 byte compression flag, 4 bytes message length

	var err error
	b, err = s.codec.MarshalAppend(b, reply)
	if err != nil {
		return err
	}

	var size uint32
	size = uint32(len(b) - 5)
	if int(size) > s.opts.maxRecvMessageSize {
		return fmt.Errorf("grpc: received message larger than max (%d vs. %d)", size, s.opts.maxRecvMessageSize)
	}

	b[0] = 0 // uncompressed
	if s.comp != nil {
		buf := buffers.Get()
		buf.Reset()
		if err := s.compress(buf, b[5:]); err != nil {
			buffers.Put(buf)
			return err
		}
		bufSize := buf.Len()
		if bufSize+5 > cap(b) {
			b = make([]byte, 0, growcap(cap(b), bufSize+5))
		}
		b = b[:bufSize+5]
		b[0] = 1 // compressed
		copy(b[5:], buf.Bytes())
		size = uint32(bufSize)
		buffers.Put(buf)
	}

	binary.BigEndian.PutUint32(b[1:], size)
	if _, err := s.w.Write(b); err != nil {
		if isStreamError(err) {
			msg := err.Error()
			return status.Errorf(codes.Unavailable, msg)
		}
		return err
	}
	s.w.(http.Flusher).Flush()
	if stats := s.opts.statsHandler; stats != nil {
		// TODO: raw payload stats.
		b := b[headerLen:] // shadow
		stats.HandleRPC(s.ctx, outPayload(false, m, b, b, time.Now()))
	}
	return nil
}

func (s *streamGRPC) decompress(dst *bytes.Buffer, b []byte) error {
	src := bytes.NewReader(b)

	r, err := s.comp.Decompress(src)
	if err != nil {
		return err
	}
	if _, err := dst.ReadFrom(r); err != nil {
		return err
	}
	return nil
}

func (s *streamGRPC) RecvMsg(m interface{}) error {
	s.wg.Add(1)
	defer s.wg.Done()

	if err := s.isDone(); err != nil {
		return err
	}

	args := m.(proto.Message)

	bp := bytesPool.Get().(*[]byte)
	b := (*bp)[:0]
	defer func() {
		if cap(b) < s.opts.maxRecvMessageSize {
			*bp = b
			bytesPool.Put(bp)
		}
	}()
	if cap(b) < 5 {
		b = make([]byte, 0, growcap(cap(b), 5))
	}
	b = b[:5] // 1 byte compression flag, 4 bytes message length

	if _, err := io.ReadFull(s.r, b); err != nil {
		if isStreamError(err) {
			msg := err.Error()
			return status.Errorf(codes.Canceled, msg)
		}
		return err
	}
	isCompressed := b[0] == 1
	size := binary.BigEndian.Uint32(b[1:])
	if int(size) > s.opts.maxRecvMessageSize {
		return fmt.Errorf("grpc: received message larger than max (%d vs. %d)", size, s.opts.maxRecvMessageSize)
	}

	if cap(b) < int(size) {
		b = make([]byte, 0, growcap(cap(b), int(size)))
	}
	b = b[:size]
	if _, err := io.ReadFull(s.r, b); err != nil {
		return err
	}

	if isCompressed {
		// compressed
		if s.comp == nil {
			return fmt.Errorf("grpc: Decompressor is not installed for grpc-encoding %q", s.messageEncoding)
		}

		buf := buffers.Get()
		buf.Reset()
		if err := s.decompress(buf, b); err != nil {
			buffers.Put(buf)
			return err
		}
		size = uint32(buf.Len())
		if int(size) > cap(b) {
			b = make([]byte, 0, growcap(cap(b), int(size)))
		}
		b = b[:int(size)]
		copy(b, buf.Bytes())
		buffers.Put(buf)
	}

	if err := s.codec.Unmarshal(b, args); err != nil {
		return err
	}
	if stats := s.opts.statsHandler; stats != nil {
		// TODO: raw payload stats.
		b := b[headerLen:] // shadow
		stats.HandleRPC(s.ctx, inPayload(false, m, b, b, time.Now()))
	}
	return nil
}

func (m *Mux) grpcGetCodec(ct string) (Codec, bool) {
	typ, enc, ok := strings.Cut(ct, "+")
	if !ok {
		enc = "proto"
	}
	if typ != "application/grpc" {
		return nil, false
	}
	c, ok := m.opts.codecsByName[enc]
	return c, ok
}

func (m *Mux) grpcGetCompressor(me string) (Compressor, bool) {
	if me == "" {
		return nil, false
	}
	c, ok := m.opts.compressors[me]
	return c, ok
}

// serveGRPC serves the gRPC server.
func (m *Mux) serveGRPC(w http.ResponseWriter, r *http.Request) {
	if r.ProtoMajor != 2 {
		msg := "gRPC requires HTTP/2"
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	if r.Method != "POST" {
		msg := fmt.Sprintf("invalid gRPC request method %q", r.Method)
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		msg := "Streaming unsupported"
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}

	contentType := r.Header.Get("Content-Type")
	codec, ok := m.grpcGetCodec(contentType)
	if !ok {
		msg := fmt.Sprintf("invalid gRPC request content-type %q", contentType)
		http.Error(w, msg, http.StatusUnsupportedMediaType)
		return
	}
	messageEncoding := r.Header.Get("Grpc-Encoding")
	var compressor Compressor
	if messageEncoding != "" {
		comp, ok := m.grpcGetCompressor(messageEncoding)
		if !ok {
			msg := fmt.Sprintf("invalid gRPC request message-encoding %q", messageEncoding)
			http.Error(w, msg, http.StatusUnsupportedMediaType)
			return
		}
		compressor = comp
	}

	ctx, md := newIncomingContext(r.Context(), r.Header)

	if v := r.Header.Get("grpc-timeout"); v != "" {
		to, err := decodeTimeout(v)
		if err != nil {
			msg := fmt.Sprintf("malformed grpc-timeout: %v", err)
			http.Error(w, msg, http.StatusBadRequest)
			return
		}
		tctx, cancel := context.WithTimeout(ctx, to)
		defer cancel()
		ctx = tctx
	}

	method := r.URL.Path
	s := m.loadState()
	hd, err := s.pickMethodHandler(method)
	if err != nil {
		msg := fmt.Sprintf("no handler for gRPC method %q", method)
		http.Error(w, msg, http.StatusNotFound)
		return
	}

	// Handle stats.
	beginTime := time.Now()
	if sh := m.opts.statsHandler; sh != nil {
		ctx = sh.TagRPC(ctx, &stats.RPCTagInfo{
			FullMethodName: hd.method,
			FailFast:       false, // TODO
		})

		sh.HandleRPC(ctx, &stats.InHeader{
			FullMethod:  method,
			RemoteAddr:  strAddr(r.RemoteAddr),
			Compression: r.Header.Get("Content-Encoding"),
			Header:      metadata.MD(md).Copy(),
		})

		sh.HandleRPC(ctx, &stats.Begin{
			Client:                    false,
			BeginTime:                 beginTime,
			FailFast:                  false, // TODO
			IsClientStream:            hd.desc.IsStreamingClient(),
			IsServerStream:            hd.desc.IsStreamingServer(),
			IsTransparentRetryAttempt: false, // TODO
		})
	}

	ctx, cancel := context.WithCancel(ctx)
	stream := &streamGRPC{
		ctx:     ctx,
		handler: hd,
		opts:    m.opts,
		codec:   codec,
		comp:    compressor,
		done:    ctx.Done(),

		// write
		w:       w,
		wHeader: w.Header(),

		// read
		r:               r.Body,
		contentType:     contentType,
		messageEncoding: messageEncoding,
		//rHeader: r.Header,
	}
	// Sync handler return to stream methods.
	defer func() {
		cancel()
		stream.wg.Wait()
	}()

	herr := hd.handler(&m.opts, stream)
	if !stream.sentHeader {
		if err := stream.SendHeader(nil); err != nil {
			return // ctx canceled
		}
	}
	flusher.Flush()
	r.Body.Close()

	// Write status.
	st := status.Convert(herr)

	h := w.Header()
	h.Set("Grpc-Status", strconv.FormatInt(int64(st.Code()), 10))
	if m := st.Message(); m != "" {
		h.Set("Grpc-Message", encodeGrpcMessage(m))
	}
	if p := st.Proto(); p != nil && len(p.Details) > 0 {
		stBytes, err := proto.Marshal(p)
		if err != nil {
			panic(err)
		}
		h.Set("Grpc-Status-Details-Bin", encodeBinHeader(stBytes))
	}
	setOutgoingHeader(h, stream.trailer)

	if sh := m.opts.statsHandler; sh != nil {
		endTime := time.Now()

		// Try to send Trailers, might not be respected.
		setOutgoingHeader(w.Header(), stream.trailer)
		sh.HandleRPC(ctx, &stats.OutTrailer{
			Trailer: stream.trailer.Copy(),
		})

		sh.HandleRPC(ctx, &stats.End{
			Client:    false,
			BeginTime: beginTime,
			EndTime:   endTime,
			Error:     herr,
		})
	}
}
