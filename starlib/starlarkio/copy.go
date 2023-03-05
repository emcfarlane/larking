package starlarkio

import (
	"io"

	"go.starlark.net/starlark"
)

func Copy(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		dstV, srcV starlark.Value
	)
	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 2, &dstV, &srcV); err != nil {
		return nil, err
	}

	dst, err := ToWriter(dstV)
	if err != nil {
		return nil, err
	}

	src, err := ToReader(dstV)
	if err != nil {
		return nil, err
	}

	n, err := io.Copy(dst, src)
	return starlark.MakeInt64(n), err
}
