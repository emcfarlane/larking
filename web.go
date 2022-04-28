package larking

// Support for gRPC-web
// https://github.com/grpc/grpc/blob/master/doc/PROTOCOL-WEB.md

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const (
	grpcWeb     = "application/grpc-web"
	grpcWebText = "application/grpc-web-text"
)

// isWebRequest checks for gRPC Web headers.
func isWebRequest(r *http.Request) (typ string, enc string, ok bool) {
	ct := r.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/grpc-web") || r.Method != http.MethodPost {
		return typ, enc, false
	}
	typ, enc, ok = strings.Cut(ct, "+")
	if !ok {
		enc = "proto"
	}
	ok = typ == grpcWeb || typ == grpcWebText
	return typ, enc, ok
}

type streamWeb struct {
	ctx context.Context
	w   http.ResponseWriter
	r   *http.Request

	typ   string // grpcWEB || grpcWebText
	enc   string // proto || json || ...
	recvN int
	sendN int

	sentHeader bool
	header     metadata.MD
	trailer    metadata.MD
}

func (s *streamWeb) SetHeader(md metadata.MD) error {
	if !s.sentHeader {
		s.header = metadata.Join(s.header, md)
	}
	return nil

}
func (s *streamWeb) SendHeader(md metadata.MD) error {
	if s.sentHeader {
		return nil // already sent?
	}
	setOutgoingHeader(s.w.Header(), s.header, md)
	s.w.WriteHeader(http.StatusOK)
	s.sentHeader = true
	return nil
}

func (s *streamWeb) SetTrailer(md metadata.MD) {
	s.trailer = metadata.Join(s.trailer, md)
}

func (s *streamWeb) Context() context.Context {
	sts := &serverTransportStream{s, s.r.URL.Path}
	return grpc.NewContextWithServerTransportStream(s.ctx, sts)
}

func (s *streamWeb) SendMsg(v interface{}) error {
	s.sendN += 1
	reply := v.(proto.Message)

	var resp io.Writer = s.w
	if s.typ == grpcWebText {
		resp = base64.NewEncoder(base64.StdEncoding, resp)
	}

	var encFn func(proto.Message) ([]byte, error)
	switch s.enc {
	case "proto":
		encFn = proto.Marshal
	case "json":
		encFn = protojson.Marshal
	default:
		return fmt.Errorf("unsupported encoding: %s", s.enc)
	}

	b, err := encFn(reply)
	if err != nil {
		return err
	}
	if _, err := io.Copy(resp, bytes.NewReader(b)); err != nil {
		return err
	}
	if fRsp, ok := s.w.(http.Flusher); ok {
		fRsp.Flush()
	}
	return nil

}

func (s *streamWeb) RecvMsg(m interface{}) error {
	s.recvN += 1
	args := m.(proto.Message)

	// TODO: compression?
	var body io.Reader = s.r.Body // body close handled by serveWeb.
	if s.typ == grpcWebText {
		body = base64.NewDecoder(base64.StdEncoding, body)
	}

	b, err := ioutil.ReadAll(io.LimitReader(body, 1024*1024*2))
	if err != nil {
		return err
	}

	switch s.enc {
	case "proto":
		if err := proto.Unmarshal(b, args); err != nil {
			return err
		}
	case "json":
		if err := protojson.Unmarshal(b, args); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported encoding: %s", s.enc)
	}
	return nil
}

func (s *streamWeb) encodeTrailer(w io.Writer) error {
	hd := make(http.Header, len(s.trailer))
	for key, val := range s.trailer {
		hd[key] = val
	}
	var buf bytes.Buffer
	if err := hd.Write(&buf); err != nil {
		return err
	}

	head := []byte{1 << 7, 0, 0, 0, 0} // MSB=1 indicates this is a trailer data frame.
	binary.BigEndian.PutUint32(head[1:5], uint32(buf.Len()))
	if _, err := w.Write(head); err != nil {
		return err
	}
	if _, err := w.Write(buf.Bytes()); err != nil {
		return err
	}
	return nil
}

func (m *Mux) serveWeb(w http.ResponseWriter, r *http.Request) (err error) {
	ctx := newIncomingContext(r.Context(), r.Header)

	typ, enc, ok := isWebRequest(r)
	if !ok {
		return fmt.Errorf("invalid gRPC-Web content type: %v", r.Header.Get("Content-Type"))
	}
	defer r.Body.Close()
	if fRsp, ok := w.(http.Flusher); ok {
		defer fRsp.Flush()
	}

	hd, err := m.loadState().pickMethodHandler(r.URL.Path)
	if err != nil {
		return err
	}

	if hd.descriptor.IsStreamingClient() || hd.descriptor.IsStreamingServer() {
		return status.Errorf(codes.Unimplemented, "streaming %s not implemented", r.URL.Path)
	}

	stream := &streamWeb{
		ctx: ctx,
		w:   w, r: r,
		typ: typ,
		enc: enc,
	}
	defer func() {
		if err == nil {
			err = stream.encodeTrailer(w)
		}
	}()

	return hd.handler(&m.opts, stream)
}
