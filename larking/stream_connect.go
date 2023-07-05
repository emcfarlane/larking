package larking

import (
	"context"
	"net/http"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	connectHeaderVersion              = "Connect-Protocol-Version"
	connectHeaderContentTypeStreaming = "application/connect"
)

// streamConnectUnary is a unary stream for connect.
type streamConnectUnary struct {
	//
}

type streamConnectStreaming struct {
	//
}

func (m *Mux) serveConnectUnary(w http.ResponseWriter, r *http.Request) {
	//
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func (m *Mux) serveConnectStreaming(w http.ResponseWriter, r *http.Request) {
	//
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func (m *Mux) serveConnect(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), connectHeaderContentTypeStreaming) {
		m.serveConnectStreaming(w, r)
		return
	}
	m.serveConnectUnary(w, r)
}

type clientStreamConnectUnary struct {
	*ClientConn

	ctx     context.Context
	method  string
	header  metadata.MD
	trailer metadata.MD

	request  *http.Request
	response *http.Response
}

var _ grpc.ClientStream = (*clientStreamConnectUnary)(nil)

func newClientStreamConnectUnary(cc *ClientConn, ctx context.Context, method string) *clientStreamConnectUnary {
	return &clientStreamConnectUnary{
		ClientConn: cc,
		ctx:        ctx,
		method:     method,
	}
}

func (cs *clientStreamConnectUnary) Header() (metadata.MD, error) {
	if cs.header == nil {
		cs.header = make(metadata.MD)
	}
	return cs.header, nil
}
func (cs *clientStreamConnectUnary) Trailer() metadata.MD {
	if cs.trailer == nil {
		cs.trailer = make(metadata.MD)
	}
	return cs.trailer
}
func (cs *clientStreamConnectUnary) CloseSend() error {
	return nil
}
func (cs *clientStreamConnectUnary) Context() context.Context {
	return cs.ctx
}
func (cs *clientStreamConnectUnary) SendMsg(m any) error {
	buf := buffers.Get()
	defer buffers.Put(buf)

	b, err := cs.Codec.MarshalAppend(buf.Bytes(), m)
	if err != nil {
		return err
	}
	writeToBuffer(buf, b)

	// if commpression
	if comp := cs.Compressor; comp != nil {
		compBuf := buffers.Get()
		if err := func() error {
			w, err := comp.Compress(compBuf)
			if err != nil {
				return err
			}
			defer w.Close()
			if _, err := w.Write(b); err != nil {
				return err
			}
			return nil
		}(); err != nil {
			buffers.Put(buf)
			return err
		}
		buf.Reset()
		compBuf.WriteTo(buf)
		buffers.Put(compBuf)
	}

	body := buf // *bytes.Buffer sets Content-Length and GetBody() returns a copy of the buffer.

	request, err := http.NewRequestWithContext(cs.ctx, "POST", cs.target+cs.method, body)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/"+cs.Codec.Name())

	response, err := cs.Transport.RoundTrip(request)
	cs.request = request
	cs.response = response
	if err != nil {
		return err
	}
	// set header/trailer from response
	return nil
}
func (cs *clientStreamConnectUnary) RecvMsg(m any) error {
	buf := buffers.Get()
	defer buffers.Put(buf)

	b, err := readAll(buf.Bytes(), cs.response.Body, -1)
	if err != nil {
		return err
	}
	writeToBuffer(buf, b)

	// if compression
	if comp := cs.Compressor; comp != nil {
		// TODO: check headers.
		compBuf := buffers.Get()

		r, err := comp.Decompress(buf)
		if err != nil {
			buffers.Put(compBuf)
			return err
		}
		buf.Reset()
		if _, err := compBuf.ReadFrom(r); err != nil {
			buffers.Put(compBuf)
			return err
		}
		buf.Reset()
		compBuf.WriteTo(buf)
		buffers.Put(compBuf)
		b = buf.Bytes()
	}

	if err := cs.Codec.Unmarshal(b, m); err != nil {
		return err
	}
	// TODO: set header/trailer
	return nil
}
