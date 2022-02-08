// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlib

import (
	"fmt"
	"net/url"
	"path"
	"strings"
)

const keyParam = "key"

func getValues(s string) (string, url.Values, error) {
	vs := strings.SplitN(s, "?", 2)
	s = vs[0]
	if len(vs) == 1 {
		return s, nil, nil
	}

	vals, err := url.ParseQuery(vs[1])
	if err != nil {
		return "", nil, err
	}
	return s, vals, nil
}

// resolveModuleURL creates a blob URL for a module string.
//
// Thread names are specialised blob urls with the key encoded as a param.
// mem://?key=current.star
//
// TODO: load syntax for git modules...
func resolveModuleURL(name string, module string) (bktURL string, pathStr string, err error) {
	var vals url.Values
	bktURL, vals, err = getValues(name)
	if err != nil {
		return
	}

	key := vals.Get(keyParam)
	dir := path.Dir(key)

	if strings.HasPrefix(module, ".") {
		fmt.Println(dir, module)
		pathStr = path.Join(".", dir, module)
	} else {
		// Resolve what bucket or if its local
		err = fmt.Errorf("TODO: resolve non-local modules: %s", module)
		return

	}

	vals.Del(keyParam)
	if len(vals) > 0 {
		bktURL += "?" + vals.Encode()
	}

	return
}

//func setKey(key string) {}
