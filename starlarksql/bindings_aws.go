// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build aws

package starlarksql

import (
	_ "gocloud.dev/mysql/awsmysql"
	_ "gocloud.dev/postgres/awspostgres"
)
