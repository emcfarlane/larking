package starlarkrule

import "testing"

func TestLabels(t *testing.T) {
	tests := []struct {
		name    string
		label   string
		dir     string
		want    string
		wantErr error
	}{{
		name:  "full",
		label: "testdata/go/hello",
		dir:   ".",
		want:  "file://testdata/go/hello",
	}, {
		name:  "full2",
		label: "testdata/go/hello",
		dir:   "",
		want:  "file://testdata/go/hello",
	}, {
		name:  "relative",
		label: "hello",
		dir:   "testdata/go",
		want:  "file://testdata/go/hello",
	}, {
		name:  "dotRelative",
		label: "./hello",
		dir:   "testdata/go",
		want:  "file://testdata/go/hello",
	}, {
		name:  "dotdotRelative",
		label: "../hello",
		dir:   "testdata/go",
		want:  "file://testdata/hello",
	}, {
		name:  "dotdotdotdotRelative",
		label: "../../hello",
		dir:   "testdata/go",
		want:  "file://hello",
	}, {
		name:  "dotdotCD",
		label: "../packaging/file",
		dir:   "testdata/go",
		want:  "file://testdata/packaging/file",
	}, {
		name:  "absolute",
		label: "/users/edward/Downloads/file.txt",
		dir:   "",
		want:  "file:///users/edward/Downloads/file.txt",
	}, {
		name:  "fileLabel",
		label: "file://rules/go/zxx",
		dir:   "testdata/cgo",
		want:  "file://rules/go/zxx",
	}, {
		name:  "queryRelative",
		label: "helloc?goarch=amd64&goos=linux",
		dir:   "testdata/cgo",
		want:  "file://testdata/cgo/helloc?goarch=amd64&goos=linux",
	}, {
		name:  "queryAbsolute",
		label: "file://testdata/cgo/helloc?goarch=amd64&goos=linux",
		dir:   "testdata/cgo",
		want:  "file://testdata/cgo/helloc?goarch=amd64&goos=linux",
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, err := ParseLabel(tt.dir, tt.label)
			if err != tt.wantErr {
				t.Fatalf("error got: %v, want: %v", err, tt.wantErr)
			}
			if l.String() != tt.want {
				t.Fatalf("%s != %s", l, tt.want)
			}
		})
	}
}
