package builder

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
	"go.starlark.net/starlark"
	_ "gocloud.dev/blob/fileblob"
	"larking.io/starlib/starlarkrule"
)

func TestRun(t *testing.T) {
	// Check local deps are available
	for _, dep := range []string{"zig"} {
		if _, err := exec.LookPath("dot"); err != nil {
			if errors.Is(err, exec.ErrNotFound) {
				t.Logf("skipping, %q needed", dep)
				t.Skip()
				return
			}
			t.Fatal(err)
		}
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
			makeDefaultInfoFiles(
				"testdata/cgo/BUILD.star",
				"testdata/cgo/helloc",
				[]string{
					"testdata/cgo/helloc",
				},
			),
		},
	}, {
		name:  "xcgo",
		label: "testdata/cgo/helloc?goarch=amd64&goos=linux",
		wants: []*starlarkrule.AttrArgs{
			makeDefaultInfoFiles(
				"testdata/cgo/BUILD.star",
				"testdata/cgo/helloc?goarch=amd64&goos=linux",
				[]string{
					"testdata/cgo/helloc?goarch=amd64&goos=linux",
				},
			),
		},
	}, {
		name:  "tar",
		label: "testdata/archive/hello.tar",
		wants: []*starlarkrule.AttrArgs{
			makeDefaultInfoFiles(
				"testdata/archive/BUILD.star", "",
				[]string{"testdata/archive/hello.tar"},
			),
		},
	}, {
		name:  "tar.gz",
		label: "testdata/archive/helloc.tar.gz",
		wants: []*starlarkrule.AttrArgs{
			makeDefaultInfoFiles(
				"testdata/archive/BUILD.star", "",
				[]string{"testdata/archive/helloc.tar.gz"},
			),
		},
	}, {
		// TODO: PULL LOCALLY
		name:  "containerPull",
		label: "testdata/container/distroless.tar",
		wants: []*starlarkrule.AttrArgs{
			makeDefaultInfoFiles(
				"testdata/container/BUILD.star", "",
				[]string{"testdata/container/distroless.tar"},
			),
		},
		//wantConstructor: imageConstructor,

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
