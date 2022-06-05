package starlarkrule

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"larking.io/starlib/starext"
	"larking.io/starlib/starlarkthread"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"gocloud.dev/blob"
)

var archiveModule = &starlarkstruct.Module{
	Name: "packaging",
	Members: starlark.StringDict{
		"tar": starext.MakeBuiltin("packaging.tar", makeTar),
	},
}

// makeTar creates a tarball.
func makeTar(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := starlarkthread.GetContext(thread)

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

	var (
		creationTime time.Time // zero
		blobs        starext.Blobs
	)
	defer blobs.Close()

	createTar := func(l *Label) error {
		fmt.Println("createTar", l)
		bktURL := l.BucketURL()
		key := l.Key()

		w, err := blobs.NewWriter(ctx, bktURL, key, nil)
		if err != nil {
			return err
		}
		defer w.Close()

		// compress writer
		var cw io.WriteCloser
		switch {
		case strings.HasSuffix(key, ".tar.gz"):
			cw = gzip.NewWriter(w)
			defer cw.Close()
		default:
			cw = w
		}

		tw := tar.NewWriter(cw)
		defer tw.Close()

		addFile := func(l *Label, key string) error {
			bktURL := l.BucketURL()
			bkt, err := blob.OpenBucket(ctx, bktURL)
			if err != nil {
				return err
			}

			r, err := blobs.NewReader(ctx, bktURL, l.Key(), nil)
			if err != nil {
				return err
			}
			defer r.Close()

			attrs, err := bkt.Attributes(ctx, l.Key())
			if err != nil {
				return err
			}

			header := &tar.Header{
				Name:     key,
				Size:     attrs.Size,
				Typeflag: tar.TypeReg,
				Mode:     0600,
				ModTime:  creationTime,
			}
			// write the header to the tarball archive
			if err := tw.WriteHeader(header); err != nil {
				return err
			}
			// copy the file data to the tarball
			if _, err := io.Copy(tw, r); err != nil {
				return err
			}
			return nil
		}

		for i, n := 0, srcs.Len(); i < n; i++ {
			src := srcs.Index(i)
			l, ok := src.(*Label)
			if !ok {
				return fmt.Errorf("invalid src type: %T", src.Type())
			}

			// Form the key path of the file in the tar fs.
			key := path.Join(
				packageDir,
				strings.TrimPrefix(l.Key(), stripPrefix),
			)

			// TODO: strip_prefix
			if err := addFile(l, key); err != nil {
				return err
			}
		}
		return nil
	}
	l, err := ParseRelativeLabel(thread.Name, name)
	if err != nil {
		return nil, err
	}

	if err := createTar(l); err != nil {
		return nil, err
	}
	fmt.Println("makeTar", l)
	return l, nil
}
