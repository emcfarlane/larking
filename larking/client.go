package larking

import (
	"context"
	"net/http"

	"google.golang.org/grpc"
)

type ClientConn struct {
	// Transport specifies the mechanism by which individual
	// HTTP requests are made.
	// If nil, DefaultTransport is used.
	Transport http.RoundTripper

	// Codec for marshaling/unmarshaling messages.
	Codec Codec
	// Compressor to use for messages.
	Compressor Compressor

	target   string
	protocol func(*ClientConn) grpc.ClientStream
}

type DialOption func(*ClientConn)

func Dial(target string, opts ...DialOption) *ClientConn {
	return &ClientConn{
		target: target,
	}
}

func (cc *ClientConn) CloseIdleConnections() {
	if t, ok := cc.transport().(interface {
		CloseIdleConnections()
	}); ok {
		t.CloseIdleConnections()
	}
}

// TODO: implement this
type CallOption = grpc.CallOption

var unaryStreamDesc = &grpc.StreamDesc{ServerStreams: false, ClientStreams: false}

func (cc *ClientConn) Invoke(ctx context.Context, method string, args, reply interface{}, opts ...CallOption) error {
	stream, err := cc.NewStream(ctx, unaryStreamDesc, method, opts...)
	if err != nil {
		return err
	}
	if err := stream.SendMsg(args); err != nil {
		return err
	}
	return stream.RecvMsg(reply)
}
func (cc *ClientConn) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...CallOption) (grpc.ClientStream, error) {
	// make stream
	cs := cc.protocol(cc)
	return cs, nil
}

func (cc *ClientConn) transport() http.RoundTripper {
	if cc.Transport != nil {
		return cc.Transport
	}
	return http.DefaultTransport
}
