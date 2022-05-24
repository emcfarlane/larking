package laze

import (
	"context"
	"testing"

	"github.com/emcfarlane/larking/starlib/starlarkrule"
	"github.com/emcfarlane/larking/starlib/starlarkstruct"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
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
	ctx := logr.NewContext(context.Background(), testr.New(t))
	b, err := NewBuilder("")
	if err != nil {
		t.Fatal(err)
	}

	a, err := b.Build(ctx, nil, "testdata/archive/helloc.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	t.Log("action", a)
	t.Log("deps", a.Deps)
	if len(a.Deps) != 1 {
		t.Errorf("missing deps: %v", a.Deps)
		t.Log("b.targetCache", b.targetCache)
		for _, tgt := range b.targetCache {
			t.Log(tgt)
		}
	}

}

func TestRun(t *testing.T) {

	type result struct {
		value starlark.Value
		error error
	}

	b, err := NewBuilder("")
	if err != nil {
		t.Fatal(err)
	}

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
	}, {
		name:  "tar",
		label: "testdata/archive/hello.tar",
		want: starlarkstruct.FromKeyValues(
			starlarkrule.Attrs,
			"file", makeLabel("testdata/archive/hello.tar"),
		),
	}, {
		name:  "tar.gz",
		label: "testdata/archive/hello.tar.gz",
		want: starlarkstruct.FromKeyValues(
			starlarkrule.Attrs,
			"file", makeLabel("testdata/archive/hello.tar.gz"),
		),

		//	name:            "xcgo",
		//	label:           "testdata/cgo/helloc?goarch=amd64&goos=linux",
		//	wantConstructor: starlarkrule.FileConstructor,
		//}, {
		//	name:            "tarxcgo",
		//	label:           "testdata/archive/helloc.tar.gz",
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
			ctx := logr.NewContext(context.Background(), testr.New(t))
			got, err := b.Build(ctx, nil, tt.label)
			if err != nil {
				t.Fatal(err)
			}
			b.Run(ctx, got)
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
				t.Log("x", x)
				t.Log("y", y)
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
