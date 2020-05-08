package graphpb

import (
	"io"
	"sync"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/dynamicpb"
)

func isStreamError(err error) bool {
	switch err {
	case nil:
		return false
	case io.EOF:
		return false
	case context.Canceled:
		return false
	}
	return true
}

// StreamHandler returns a gRPC stream handler to proxy gRPC requests.
func (m *Mux) StreamHandler() grpc.StreamHandler {
	return func(srv interface{}, stream grpc.ServerStream) error {
		ctx := stream.Context()
		name, _ := grpc.Method(ctx)

		mc, err := m.pickMethodConn(name)
		if err != nil {
			return err
		}

		// TODO: non marhsalling codec
		argsDesc := mc.desc.Input()
		replyDesc := mc.desc.Output()

		args := dynamicpb.NewMessage(argsDesc)
		reply := dynamicpb.NewMessage(replyDesc)

		if err := stream.RecvMsg(args); err != nil {
			return err
		}

		md, _ := metadata.FromIncomingContext(ctx)
		ctx = metadata.NewOutgoingContext(ctx, md)

		sd := &grpc.StreamDesc{
			ServerStreams: mc.desc.IsStreamingServer(),
			ClientStreams: mc.desc.IsStreamingClient(),
		}

		clientStream, err := mc.cc.NewStream(ctx, sd, name)
		if err != nil {
			return err
		}

		if err := clientStream.SendMsg(args); err != nil {
			return err
		}

		var inErr error
		var wg sync.WaitGroup
		if sd.ClientStreams {
			wg.Add(1)
			go func() {
				for {
					if inErr = stream.RecvMsg(args); inErr != nil {
						break
					}

					if inErr = clientStream.SendMsg(args); inErr != nil {
						break
					}
				}
				wg.Done()
			}()
		}

		var outErr error
		for {
			if outErr = clientStream.RecvMsg(reply); outErr != nil {
				break
			}

			if outErr = stream.SendMsg(reply); outErr != nil {
				break
			}

			if !sd.ServerStreams {
				break
			}
		}

		if isStreamError(outErr) {
			return outErr
		}

		if sd.ClientStreams {
			wg.Wait()
			if isStreamError(inErr) {
				return inErr
			}
		}

		trailer := clientStream.Trailer()
		stream.SetTrailer(trailer)

		return nil
	}
}
