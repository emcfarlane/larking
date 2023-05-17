package larking

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gobwas/ws"
	"google.golang.org/genproto/googleapis/rpc/code"
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

var _ grpc.ServerStream = (*streamHTTP)(nil)

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

	h := s.wHeader
	setOutgoingHeader(h, s.header)
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
		h := s.wHeader
		h.Set("Content-Type", contentType)
		if !s.sentHeader {
			if err := s.SendHeader(nil); err != nil {
				return count, err
			}
		}
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
		fds := cur.Descriptor().Fields()
		fdContentType := fds.ByName("content_type")
		fdData := fds.ByName("data")
		cur.Set(fdContentType, protoreflect.ValueOfString(s.contentType))

		cpy := make([]byte, len(b))
		copy(cpy, b)
		cur.Set(fdData, protoreflect.ValueOfBytes(cpy))
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

func isWebsocketRequest(r *http.Request) bool {
	for _, header := range r.Header["Upgrade"] {
		if header == "websocket" {
			return true
		}
	}
	return false
}

type twirpError struct {
	Code    string            `json:"code"`
	Message string            `json:"msg"`
	Meta    map[string]string `json:"meta"`
}

func (m *Mux) encError(w http.ResponseWriter, r *http.Request, err error) {
	s, _ := status.FromError(err)
	if isTwirp := r.Header.Get("Twirp-Version") != ""; isTwirp {
		accept := "application/json"

		w.Header().Set("Content-Type", accept)
		w.WriteHeader(HTTPStatusCode(s.Code()))

		codeStr := strings.ToLower(code.Code_name[int32(s.Code())])

		terr := &twirpError{
			Code:    codeStr,
			Message: s.Message(),
		}
		b, err := json.Marshal(terr)
		if err != nil {
			panic(err) // ...
		}
		w.Write(b) //nolint
		return

	}

	accept := negotiateContentType(r.Header, m.opts.contentTypeOffers, "application/json")
	c := m.opts.codecs[accept]

	w.Header().Set("Content-Type", accept)
	w.WriteHeader(HTTPStatusCode(s.Code()))

	b, err := c.Marshal(s.Proto())
	if err != nil {
		panic(err) // ...
	}
	w.Write(b) //nolint
}

func (m *Mux) serveHTTP(w http.ResponseWriter, r *http.Request) error {
	ctx, mdata := newIncomingContext(r.Context(), r.Header)

	s := m.loadState()
	isWebsocket := isWebsocketRequest(r)

	verb := r.Method
	if isWebsocket {
		verb = kindWebsocket
	}

	method, params, err := s.match(r.URL.Path, verb)
	if err != nil {
		return err
	}

	queryParams, err := method.parseQueryParams(r.URL.Query())
	if err != nil {
		return err
	}
	params = append(params, queryParams...)

	hd, err := s.pickMethodHandler(method.name)
	if err != nil {
		return err
	}

	// Handle stats.
	beginTime := time.Now()
	if sh := m.opts.statsHandler; sh != nil {
		ctx = sh.TagRPC(ctx, &stats.RPCTagInfo{
			FullMethodName: hd.method,
			FailFast:       false, // TODO
		})

		sh.HandleRPC(ctx, &stats.InHeader{
			FullMethod:  method.name,
			RemoteAddr:  strAddr(r.RemoteAddr),
			Compression: r.Header.Get("Content-Encoding"),
			Header:      metadata.MD(mdata).Copy(),
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

	if isWebsocket {
		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			return err
		}
		defer conn.Close()

		stream := &streamWS{
			ctx:    ctx,
			conn:   conn,
			method: method,
			params: params,
		}
		herr := hd.handler(&m.opts, stream)

		if herr != nil {
			s, _ := status.FromError(herr)
			// TODO: limit message size.

			code := WSStatusCode(s.Code())
			f := ws.NewCloseFrame(ws.NewCloseFrameBody(code, s.Message()))
			b, err := ws.CompileFrame(f)
			if err != nil {
				return err
			}
			if _, err := conn.Write(b); err != nil {
				return err
			}
		} else {
			if _, err := conn.Write(ws.CompiledClose); err != nil {
				return err
			}
		}

		// Handle stats.
		if sh := m.opts.statsHandler; sh != nil {
			endTime := time.Now()
			sh.HandleRPC(ctx, &stats.End{
				Client:    false,
				BeginTime: beginTime,
				EndTime:   endTime,
				Error:     err,
			})
		}
		return nil
	}

	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	contentEncoding := r.Header.Get("Content-Encoding")

	var body io.Reader = r.Body
	if cz := m.opts.compressors[contentEncoding]; cz != nil {
		z, err := cz.Decompress(r.Body)
		if err != nil {
			return err
		}
		body = z
	}

	accept := negotiateContentType(r.Header, m.opts.contentTypeOffers, contentType)
	acceptEncoding := negotiateContentEncoding(r.Header, m.opts.encodingTypeOffers)

	var resp io.Writer = w
	if cz := m.opts.compressors[acceptEncoding]; cz != nil {
		w.Header().Set("Content-Encoding", acceptEncoding)
		z, err := cz.Compress(w)
		if err != nil {
			return err
		}
		defer z.Close()
		resp = z
	}

	stream := &streamHTTP{
		ctx:    ctx,
		method: method,
		params: params,
		opts:   m.opts,

		// write
		w:       resp,
		wHeader: w.Header(),

		// read
		r:       body,
		rHeader: r.Header,

		contentType:    contentType,
		accept:         accept,
		acceptEncoding: acceptEncoding,
		hasBody:        r.ContentLength > 0 || r.ContentLength == -1,
	}
	herr := hd.handler(&m.opts, stream)
	// Handle stats.
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
	if herr != nil {
		if !stream.sentHeader {
			w.Header().Set("Content-Encoding", "identity") // try to avoid gzip
		}
		m.encError(w, r, herr)
	}
	return nil
}

func streamHTTPFromCtx(ctx context.Context) (*streamHTTP, error) {
	ss := grpc.ServerTransportStreamFromContext(ctx)
	if ss == nil {
		return nil, fmt.Errorf("invalid server transport stream")
	}
	sts, ok := ss.(*serverTransportStream)
	if !ok {
		return nil, fmt.Errorf("unknown server transport stream")
	}
	s, ok := sts.ServerStream.(*streamHTTP)
	if !ok {
		return nil, fmt.Errorf("expected HTTP stream got %T", sts.ServerStream)
	}
	return s, nil
}

// AsHTTPBodyReader returns the reader of a stream of google.api.HttpBody.
// The first message will be unmarshalled into msg excluding the data field.
// The returned reader is only valid during the lifetime of the RPC.
func AsHTTPBodyReader(stream grpc.ServerStream, msg proto.Message) (body io.Reader, err error) {
	ctx := stream.Context()
	s, err := streamHTTPFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	if !s.method.desc.IsStreamingClient() {
		return nil, fmt.Errorf("expected streaming client")
	}
	if s.recvCount > 0 {
		return nil, fmt.Errorf("expected first message")
	}

	cur := msg.ProtoReflect()
	if name, want := cur.Descriptor().FullName(), s.method.desc.Input().FullName(); name != want {
		return nil, fmt.Errorf("expected %s got %s", want, name)
	}
	for _, fd := range s.method.body {
		cur = cur.Mutable(fd).Message()
	}

	if typ := cur.Descriptor().FullName(); typ != "google.api.HttpBody" {
		return nil, fmt.Errorf("expected body type of google.api.HttpBody got %s", typ)
	}

	fds := cur.Descriptor().Fields()
	fdContentType := fds.ByName("content_type")
	cur.Set(fdContentType, protoreflect.ValueOfString(s.contentType))
	// TODO: extensions?

	if err := s.params.set(msg); err != nil {
		return nil, err
	}
	s.recvCount += 1
	return s.r, nil
}

// AsHTTPBodyWriter returns the writer of a stream of google.api.HttpBody.
// The first message will be marshalled from msg excluding the data field.
// The returned writer is only valid during the lifetime of the RPC.
func AsHTTPBodyWriter(stream grpc.ServerStream, msg proto.Message) (body io.Writer, err error) {
	ctx := stream.Context()
	s, err := streamHTTPFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	if !s.method.desc.IsStreamingServer() {
		return nil, fmt.Errorf("expected streaming server")
	}
	if s.sendCount > 0 {
		return nil, fmt.Errorf("expected first message")
	}

	cur := msg.ProtoReflect()
	if name, want := cur.Descriptor().FullName(), s.method.desc.Output().FullName(); name != want {
		return nil, fmt.Errorf("expected %s got %s", want, name)
	}
	for _, fd := range s.method.resp {
		cur = cur.Mutable(fd).Message()
	}

	if typ := cur.Descriptor().FullName(); typ != "google.api.HttpBody" {
		return nil, fmt.Errorf("expected body type of google.api.HttpBody got %s", typ)
	}

	fds := cur.Descriptor().Fields()
	fdContentType := fds.ByName("content_type")
	pContentType := cur.Get(fdContentType)
	contentType := pContentType.String()

	s.wHeader.Set("Content-Type", contentType)
	if !s.sentHeader {
		if err := s.SendHeader(nil); err != nil {
			return nil, err
		}
	}
	s.sendCount += 1
	return s.w, nil
}
