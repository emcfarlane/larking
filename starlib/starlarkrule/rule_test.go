package starlarkrule

import (
	"context"
	"net/url"
	"testing"

	"gocloud.dev/blob"
	_ "gocloud.dev/blob/fileblob"
)

func TestLabels(t *testing.T) {
	tests := []struct {
		name    string
		label   string
		source  string
		want    string
		wantErr error
	}{{
		name:   "full",
		label:  "testdata/go/hello",
		source: "file:///", // root
		want: "file:///?" + url.Values{
			"key": []string{"testdata/go/hello"},
		}.Encode(),
	}, {
		name:   "relative",
		label:  "hello",
		source: "file://./",
		want: "file://./?" + url.Values{
			"key": []string{"hello"},
		}.Encode(),
	}, {
		name:  "dotRelative",
		label: "./hello",
		source: "file://./?" + url.Values{
			"key": []string{"testdata/go/file.star"},
		}.Encode(),
		want: "file://./?" + url.Values{
			"key": []string{"testdata/go/hello"},
		}.Encode(),
	}, {
		name:  "dotdotRelative",
		label: "../hello",
		source: "file://./?" + url.Values{
			"key": []string{"testdata/go/file.star"},
		}.Encode(),
		want: "file://./?" + url.Values{
			"key": []string{"testdata/hello"},
		}.Encode(),
	}, {
		name:  "dotdotdotdotRelative",
		label: "../../hello",
		source: "file://./?" + url.Values{
			"key": []string{"testdata/go/file.star"},
		}.Encode(),
		want: "file://./?" + url.Values{
			"key": []string{"hello"},
		}.Encode(),
	}, {
		name:  "dotdotCD",
		label: "../packaging/file.star",
		source: "file://./?" + url.Values{
			"key": []string{"testdata/go/file.star"},
		}.Encode(),
		want: "file://./?" + url.Values{
			"key": []string{"testdata/packaging/file.star"},
		}.Encode(),
	}, {
		name:  "other",
		label: "file:///usr/local/bin?key=go",
		source: "file://./?" + url.Values{
			"key": []string{"testdata/go/file.star"},
		}.Encode(),
		want: "file:///usr/local/bin?key=go",
		/*}, {
			name:  "absolute",
			label: "/users/edward/Downloads/file.txt",
			dir:   "",
			want:  "file:///users/edward/Downloads/file.txt",
		}, {
			name:  "fileLabel",
			label: "file://rules/go/zxx",
			dir:   "testdata/cgo",
			want:  "file://rules/go/zxx",
		*/
		// TODO: query parameters as part of the binary?
		/*}, {
			name:  "queryRelative",
			label: "helloc?goarch=amd64&goos=linux",
			source: "file://./?" + url.Values{
				"key": []string{"testdata/go/file.star"},
			}.Encode(),
			want: "file://./?" + url.Values{
				"key":    []string{"testdata/go/helloc"},
				"goarch": []string{"amd64"},
				"goos":   []string{"linux"},
			}.Encode(),
		}, {
			name: "queryAbsoluteURL",
			label: "file://./?" + url.Values{
				"key":    []string{"testdata/cgo/helloc"},
				"goarch": []string{"amd64"},
				"goos":   []string{"linux"},
			}.Encode(),
			source: "file://./?" + url.Values{
				"key": []string{"testdata/go/file.star"},
			}.Encode(),
			want: "file://./?" + url.Values{
				"key":    []string{"testdata/go/helloc"},
				"goarch": []string{"amd64"},
				"goos":   []string{"linux"},
			}.Encode(),*/
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, err := ParseLabel(tt.source, tt.label)
			if err != tt.wantErr {
				t.Fatalf("error got: %v, want: %v", err, tt.wantErr)
			}
			if l.String() != tt.want {
				t.Fatalf("%s != %s", l, tt.want)
			}

			// Check valid bucket.
			ctx := context.Background()
			bkt, err := blob.OpenBucket(ctx, l.String())
			if err != nil {
				t.Fatal(err)
			}
			defer bkt.Close()
		})
	}
}
