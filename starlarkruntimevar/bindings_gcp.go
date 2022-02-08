// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build gcp

package starlarkruntimevar

import (
	_ "gocloud.dev/runtimevar/gcpruntimeconfig"
	_ "gocloud.dev/runtimevar/gcpsecretmanager"
)
