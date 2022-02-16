// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build gcp

package bindings

import (
	_ "gocloud.dev/blob/gcsblob"
	_ "gocloud.dev/mysql/gcpmysql"
	_ "gocloud.dev/postgres/gcppostgres"
	_ "gocloud.dev/runtimevar/gcpruntimeconfig"
	_ "gocloud.dev/runtimevar/gcpsecretmanager"
)
