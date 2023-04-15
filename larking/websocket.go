package larking

import (
	"context"
	"net"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const kindWebsocket = "WEBSOCKET"

type streamWS struct {
	ctx  context.Context
	conn net.Conn

	method *method
	params params
	recvN  int
	sendN  int

	sentHeader bool
	header     metadata.MD
	trailer    metadata.MD
}

func (s *streamWS) SetHeader(md metadata.MD) error {
	if !s.sentHeader {
		s.header = metadata.Join(s.header, md)
	}
	return nil

}
func (s *streamWS) SendHeader(md metadata.MD) error {
	if s.sentHeader {
		return nil // already sent?
	}
	// TODO: headers?
	s.sentHeader = true
	return nil
}

func (s *streamWS) SetTrailer(md metadata.MD) {
	s.sentHeader = true
	s.trailer = metadata.Join(s.trailer, md)
}

func (s *streamWS) Context() context.Context {
	sts := &serverTransportStream{s, s.method.name}
	return grpc.NewContextWithServerTransportStream(s.ctx, sts)
}

func (s *streamWS) SendMsg(v interface{}) error {
	s.sendN += 1
	reply := v.(proto.Message)
	//ctx := s.ctx

	cur := reply.ProtoReflect()
	for _, fd := range s.method.resp {
		cur = cur.Mutable(fd).Message()
	}
	msg := cur.Interface()

	// TODO: contentType check?
	b, err := protojson.Marshal(msg)
	if err != nil {
		return err
	}

	if err := wsutil.WriteServerMessage(s.conn, ws.OpText, b); err != nil {
		return err
	}
	return nil
}

func (s *streamWS) RecvMsg(m interface{}) error {
	s.recvN += 1
	args := m.(proto.Message)

	if s.method.hasBody {
		cur := args.ProtoReflect()
		for _, fd := range s.method.body {
			cur = cur.Mutable(fd).Message()
		}

		msg := cur.Interface()

		b, _, err := wsutil.ReadClientData(s.conn)
		if err != nil {
			return err
		}

		// TODO: contentType check?
		// What marshalling options should we support?
		if err := protojson.Unmarshal(b, msg); err != nil {
			return err
		}
	}

	if s.recvN == 1 {
		if err := s.params.set(args); err != nil {
			return err
		}
	}
	return nil
}
