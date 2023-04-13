// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package larking

import (
	"net/http"

	"google.golang.org/grpc/codes"
	"nhooyr.io/websocket"
)

var codeToHTTPStatus = [...]int{
	http.StatusOK,                  // 0
	http.StatusRequestTimeout,      // 1
	http.StatusInternalServerError, // 2
	http.StatusBadRequest,          // 3
	http.StatusGatewayTimeout,      // 4
	http.StatusNotFound,            // 5
	http.StatusConflict,            // 6
	http.StatusForbidden,           // 7
	http.StatusTooManyRequests,     // 8
	http.StatusBadRequest,          // 9
	http.StatusConflict,            // 10
	http.StatusBadRequest,          // 11
	http.StatusNotImplemented,      // 12
	http.StatusInternalServerError, // 13
	http.StatusServiceUnavailable,  // 14
	http.StatusInternalServerError, // 15
	http.StatusUnauthorized,        // 16
}

func HTTPStatusCode(c codes.Code) int {
	if int(c) > len(codeToHTTPStatus) {
		return http.StatusInternalServerError
	}
	return codeToHTTPStatus[c]
}

// TODO: validate error codes.
var codeToWSStatus = [...]websocket.StatusCode{
	websocket.StatusNormalClosure,   // 0
	websocket.StatusGoingAway,       // 1
	websocket.StatusInternalError,   // 2
	websocket.StatusUnsupportedData, // 3
	websocket.StatusGoingAway,       // 4
	websocket.StatusInternalError,   // 5
	websocket.StatusInternalError,   // 6
	websocket.StatusInternalError,   // 7
	websocket.StatusInternalError,   // 8
	websocket.StatusInternalError,   // 9
	websocket.StatusInternalError,   // 10
	websocket.StatusInternalError,   // 11
	websocket.StatusUnsupportedData, // 12
	websocket.StatusInternalError,   // 13
	websocket.StatusInternalError,   // 14
	websocket.StatusInternalError,   // 15
	websocket.StatusPolicyViolation, // 16
}

func WSStatusCode(c codes.Code) websocket.StatusCode {
	if int(c) > len(codeToHTTPStatus) {
		return websocket.StatusInternalError
	}
	return codeToWSStatus[c]
}
