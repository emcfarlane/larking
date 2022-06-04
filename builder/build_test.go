package builder

import (
	"context"
	"testing"

	"github.com/emcfarlane/larking/starlib/starlarkrule"
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

/*func TestBuild(t *testing.T) {
	ctx := logr.NewContext(context.Background(), testr.New(t))
	a, err := Build(ctx, "testdata/archive/helloc.tar.gz", func(o *buildOptions) {
		o.run = false
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Log("action", a)
	t.Log("deps", a.Deps)
	if len(a.Deps) != 1 {
		t.Errorf("missing deps: %v", a.Deps)
		//	t.Log("b.targetCache", b.targetCache)
		//	for _, tgt := range b.targetCache {
		//		t.Log(tgt)
		//	}
	}

}*/

func TestRun(t *testing.T) {

	type result struct {
		value starlark.Value
		error error
	}

	src := "file://./?metadata=skip"
	makeLabel := func(name string) *starlarkrule.Label {
		l, err := starlarkrule.ParseRelativeLabel(src, name)
		if err != nil {
			t.Fatal(err)
		}
		return l
	}
	srcLabel := makeLabel(src)

	makeDefaultInfoFiles := func(modSrc string, executable string, filenames []string) *starlarkrule.AttrArgs {
		var files []starlark.Value
		for _, filename := range filenames {
			files = append(files, makeLabel(filename))
		}
		var kwargs []starlark.Tuple
		if len(files) > 0 {
			kwargs = append(kwargs, starlark.Tuple{
				starlark.String("files"),
				starlark.NewList(files),
			})
		}

		if len(executable) > 0 {
			kwargs = append(kwargs, starlark.Tuple{
				starlark.String("executable"),
				makeLabel(executable),
			})
		}

		srcLabel, err := srcLabel.Parse(modSrc)
		if err != nil {
			t.Fatal(err)
		}

		args, err := starlarkrule.DefaultInfo.MakeArgs(srcLabel, kwargs)
		if err != nil {
			t.Fatal(err)
		}
		return args
	}

	tests := []struct {
		name    string
		label   string
		wants   []*starlarkrule.AttrArgs // starlark.Value
		wantErr error
	}{{
		name:  "cgo",
		label: "testdata/cgo/helloc",
		wants: []*starlarkrule.AttrArgs{
			makeDefaultInfoFiles("testdata/cgo/BUILD.star", "testdata/cgo/helloc", nil),
		},
	}, {
		name:  "tar",
		label: "testdata/archive/hello.tar",
		wants: []*starlarkrule.AttrArgs{
			makeDefaultInfoFiles("testdata/archive/BUILD.star", "", []string{"testdata/archive/hello.tar"}),
		},
	}, {
		//			name:  "tar.gz",
		//			label: "testdata/archive/hello.tar.gz",
		//			want: starlarkstruct.FromKeyValues(
		//				Attrs,
		//				"file", makeLabel("testdata/archive/hello.tar.gz"),
		//			),
		//
		//	name:            "xcgo",
		//	label:           "testdata/cgo/helloc?goarch=amd64&goos=linux",
		//	wantConstructor: FileConstructor,
		//}, {
		//	name:            "tarxcgo",
		//	label:           "testdata/archive/helloc.tar.gz",
		//	wantConstructor: FileConstructor,
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
			got, err := Build(ctx, tt.label)
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
			//if x := tt.want; x != nil {
			//	y := got.Value
			//	t.Log("x", x)
			//	t.Log("y", y)
			//	ok, err := starlark.Equal(x, y)
			//	if err != nil {
			//		t.Fatal(err)
			//	}
			//	if !ok {
			//		t.Errorf("%v != %v", x, y)
			//	}
			//}

			for _, want := range tt.wants {
				attrs := want.Attrs()
				v, ok, err := got.Get(attrs)
				if err != nil {
					t.Fatal(err)
				}
				if !ok {
					t.Errorf("missing attrs: %v", attrs)
					continue
				}

				t.Logf("GOT!\t\t %v %T", v, v)
				t.Logf("WANT\t\t %v %T", want, want)
				ok, err = starlark.Equal(v, want)
				if err != nil {
					t.Fatal(err)
				}
				if !ok {
					t.Errorf("not equal, got %v, want %v", v, want)
				}
				t.Logf("%v", want)
			}
		})
	}
}
