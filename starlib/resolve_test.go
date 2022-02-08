// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlib

import (
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {

	for _, tt := range []struct {
		name       string
		threadName string
		module     string
		wantBkt    string
		wantPath   string
	}{{
		name:       "local",
		threadName: "file://?prefix=a%2Fsubfolder%2F",
		module:     "./mod.star",
		wantBkt:    "file://?prefix=a/subfolder/",
		wantPath:   "mod.star",
	}, {
		name:       "localKey",
		threadName: "file://?prefix=a/subfolder/&key=b/nested/file.star",
		module:     "./mod.star",
		wantBkt:    "file://?prefix=a/subfolder/",
		wantPath:   "b/nested/mod.star",
	}, {
		name:       "parentKey",
		threadName: "file://?prefix=a/subfolder/&key=b/nested/file.star",
		module:     "../mod.star",
		wantBkt:    "file://?prefix=a/subfolder/",
		wantPath:   "b/mod.star",
	}} {
		t.Run(tt.name, func(t *testing.T) {
			gotBkt, gotPath, err := resolveModuleURL(tt.threadName, tt.module)
			if err != nil {
				t.Fatal(err)
			}
			gotBkt = strings.ReplaceAll(gotBkt, "%2F", "/") // url encoding
			if gotBkt != tt.wantBkt {
				t.Errorf("invalid bkt: %s, want %s", gotBkt, tt.wantBkt)
			}
			if gotPath != tt.wantPath {
				t.Errorf("invalid path: %s, want %s", gotPath, tt.wantPath)
			}

		})
	}
}
