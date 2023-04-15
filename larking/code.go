// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package larking

import (
	"net/http"

	"github.com/gobwas/ws"
	"google.golang.org/grpc/codes"
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
var codeToWSStatus = [...]ws.StatusCode{
	ws.StatusNormalClosure,       // 0
	ws.StatusGoingAway,           // 1
	ws.StatusInternalServerError, // 2
	ws.StatusUnsupportedData,     // 3
	ws.StatusGoingAway,           // 4
	ws.StatusInternalServerError, // 5
	ws.StatusGoingAway,           // 6
	ws.StatusInternalServerError, // 7
	ws.StatusInternalServerError, // 8
	ws.StatusInternalServerError, // 9
	ws.StatusInternalServerError, // 10
	ws.StatusInternalServerError, // 11
	ws.StatusUnsupportedData,     // 12
	ws.StatusInternalServerError, // 13
	ws.StatusInternalServerError, // 14
	ws.StatusInternalServerError, // 15
	ws.StatusPolicyViolation,     // 16
}

func WSStatusCode(c codes.Code) ws.StatusCode {
	if int(c) > len(codeToHTTPStatus) {
		return ws.StatusInternalServerError
	}
	return codeToWSStatus[c]
}
