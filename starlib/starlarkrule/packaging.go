package starlarkrule

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/emcfarlane/larking/starlib/starext"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

var packagingModule = &starlarkstruct.Module{
	Name: "packaging",
	Members: starlark.StringDict{
		"tar": starext.MakeBuiltin("packaging.tar", makeTar),
	},
}

// makeTar creates a tarball.
func makeTar(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		name        string
		srcs        *starlark.List
		packageDir  string
		stripPrefix string
	)
	if err := starlark.UnpackArgs(
		fnname, args, kwargs,
		"name", &name,
		"srcs", &srcs,
		"package_dir?", &packageDir,
		"strip_prefix?", &stripPrefix,
	); err != nil {
		return nil, err
	}

	creationTime := time.Time{} // zero
	filename := ""              // p.key

	createTar := func(filename string) error {
		f, err := os.Create(filename)
		if err != nil {
			return err
		}
		defer f.Close()

		// compress writer
		var cw io.WriteCloser
		switch {
		case strings.HasSuffix(filename, ".tar.gz"):
			cw = gzip.NewWriter(f)
			defer cw.Close()
		default:
			cw = f
		}

		tw := tar.NewWriter(cw)
		defer tw.Close()

		addFile := func(filename, key string) error {
			file, err := os.Open(filename)
			if err != nil {
				return err
			}
			defer file.Close()

			stat, err := file.Stat()
			if err != nil {
				return err
			}

			header := &tar.Header{
				Name:     key,
				Size:     stat.Size(),
				Typeflag: tar.TypeReg,
				Mode:     int64(stat.Mode()),
				ModTime:  creationTime,
			}
			// write the header to the tarball archive
			if err := tw.WriteHeader(header); err != nil {
				return err
			}
			// copy the file data to the tarball
			if _, err := io.Copy(tw, file); err != nil {
				return err
			}
			return nil
		}

		iter := srcs.Iterate()
		defer iter.Done()

		var x starlark.Value
		for iter.Next(&x) {
			fileProvider, err := toStruct(x, FileConstructor)
			if err != nil {
				return err
			}

			filename, err := getAttrStr(fileProvider, "path")
			if err != nil {
				return err
			}
			name, err := getAttrStr(fileProvider, "name")
			if err != nil {
				return err
			}

			// Form the key path of the file in the tar fs.
			key := path.Join(packageDir, strings.TrimPrefix(name, stripPrefix))

			// TODO: strip_prefix
			if err := addFile(filename, key); err != nil {
				return err
			}
		}
		return nil
	}
	if err := createTar(filename); err != nil {
		return nil, err
	}

	fi, err := os.Stat(filename)
	if err != nil {
		return nil, err
	}
	return NewFile(filename, fi)
}
