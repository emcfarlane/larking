package laze

import (
	"context"
	"testing"

	"github.com/emcfarlane/larking/starlib/starlarkrule"
	"github.com/emcfarlane/larking/starlib/starlarkstruct"
	"go.starlark.net/starlark"
	_ "gocloud.dev/blob/fileblob"
)

//func (b *Builder) testFile(
//	basename string,
//	dirname string,
//	extension string,
//	filename string,
//	isDir bool,
//	size int64,
//) starlark.Value {
//	return starlarkstruct.FromStringDict(fileConstructor, starlark.StringDict{
//		"basename":     starlark.String(basename),
//		"dirname":      starlark.String(dirname),
//		"extension":    starlark.String(extension),
//		"filename":     starlark.String(filename),
//		"is_directory": starlark.Bool(isDir),
//		"size":         starlark.MakeInt64(size),
//	})
//}

func TestBuild(t *testing.T) {

	type result struct {
		value starlark.Value
		error error
	}

	b := NewBuilder("")

	src := "file://./?metadata=skip"
	makeLabel := func(name string) *starlarkrule.Label {
		l, err := starlarkrule.ParseLabel(src, name)
		if err != nil {
			t.Fatal(err)
		}
		return l
	}

	tests := []struct {
		name    string
		label   string
		want    starlark.Value
		wantErr error
	}{{
		//name:            "merge",
		//label:           "testdata/merge/merge",
		//wantConstructor: starlarkrule.FileConstructor,
		//}, {
		name:  "cgo",
		label: "testdata/cgo/helloc",
		want: starlarkstruct.FromKeyValues(
			starlarkrule.Attrs,
			"bin", makeLabel("testdata/cgo/helloc"),
		),
		//}, {
		//	name:            "xcgo",
		//	label:           "testdata/cgo/helloc?goarch=amd64&goos=linux",
		//	wantConstructor: starlarkrule.FileConstructor,
		//}, {
		//	name:            "tarxcgo",
		//	label:           "testdata/packaging/helloc.tar.gz",
		//	wantConstructor: starlarkrule.FileConstructor,
		//}, {
		//	name:            "containerPull",
		//	label:           "testdata/container/distroless.tar",
		//	wantConstructor: imageConstructor,
		//}, {
		//	name:            "containerBuild",
		//	label:           "testdata/container/helloc.tar",
		//	wantConstructor: imageConstructor,
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			got, err := b.Build(ctx, nil, tt.label)
			if err != nil {
				t.Fatal(err)
			}
			if got.Failed && got.Error != tt.wantErr {
				t.Fatalf("error got: %v, want: %v", got.Error, tt.wantErr)
			}
			if got.Failed {
				t.Fatal("error failed: ", got)
			}
			t.Log("GOT", got.Label)
			t.Log("GOT", got.Value)
			t.Log("GOT", got.Error)
			if x := tt.want; x != nil {
				y := got.Value
				ok, err := starlark.Equal(x, y)
				if err != nil {
					t.Fatal(err)
				}
				if !ok {
					t.Errorf("%v != %v", x, y)
				}
			}

			///if c := tt.wantConstructor; c != nil {
			///	_, err := got.loadStructValue(c)
			///	if err != nil {
			///		t.Fatalf("error value: %v", err)
			///	}
			///}
		})
	}

}
