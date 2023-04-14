// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package larking

import (
	"time"

	"google.golang.org/grpc/stats"
)

const (
	payloadLen = 1
	sizeLen    = 4
	headerLen  = payloadLen + sizeLen
)

func outPayload(client bool, msg interface{}, data, payload []byte, t time.Time) *stats.OutPayload {
	return &stats.OutPayload{
		Client:     client,
		Payload:    msg,
		Data:       data,
		Length:     len(data),
		WireLength: len(payload) + headerLen,
		SentTime:   t,
	}
}

func inPayload(client bool, msg interface{}, data, payload []byte, t time.Time) *stats.InPayload {
	return &stats.InPayload{
		Client:     true,
		RecvTime:   t,
		Payload:    msg,
		Data:       data,
		Length:     len(data),
		WireLength: len(payload) + headerLen,
	}
}

// strAddr is a net.Addr backed by either a TCP "ip:port" string, or
// the empty string if unknown.
type strAddr string

func (a strAddr) Network() string {
	if a != "" {
		// Per the documentation on net/http.Request.RemoteAddr, if this is
		// set, it's set to the IP:port of the peer (hence, TCP):
		// https://golang.org/pkg/net/http/#Request
		//
		// If we want to support Unix sockets later, we can
		// add our own grpc-specific convention within the
		// grpc codebase to set RemoteAddr to a different
		// format, or probably better: we can attach it to the
		// context and use that from serverHandlerTransport.RemoteAddr.
		return "tcp"
	}
	return ""
}
func (a strAddr) String() string { return string(a) }
