// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build aws

package bindings

import (
	_ "gocloud.dev/blob/s3blob"
	_ "gocloud.dev/mysql/awsmysql"
	_ "gocloud.dev/postgres/awspostgres"
	_ "gocloud.dev/runtimevar/awsparamstore"
	_ "gocloud.dev/runtimevar/awssecretsmanager"
)
