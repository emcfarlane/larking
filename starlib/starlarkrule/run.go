package starlarkrule

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"

	"github.com/emcfarlane/larking/starlib/starlarkthread"
	"go.starlark.net/starlark"
)

func run(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		name    string
		dir     string
		argList *starlark.List
		envList *starlark.List
	)

	if err := starlark.UnpackArgs(
		fnname, args, kwargs,
		"name", &name, "dir", &dir, "args", &argList, "env?", &envList,
	); err != nil {
		return nil, err
	}

	var (
		x       starlark.Value
		cmdArgs []string
		cmdEnv  []string
	)

	iter := argList.Iterate()
	for iter.Next(&x) {
		s, ok := starlark.AsString(x)
		if !ok {
			return nil, fmt.Errorf("error: unexpected run arg: %v", x)
		}
		cmdArgs = append(cmdArgs, s)
	}
	iter.Done()

	iter = envList.Iterate()
	for iter.Next(&x) {
		s, ok := starlark.AsString(x)
		if !ok {
			return nil, fmt.Errorf("error: unexpected run env: %v", x)
		}
		cmdEnv = append(cmdEnv, s)
	}
	iter.Done()

	ctx := starlarkthread.Context(thread)
	cmd := exec.CommandContext(ctx, name, cmdArgs...)
	cmd.Dir = path.Dir(dir)
	cmd.Env = append(os.Environ(), cmdEnv...)

	var output bytes.Buffer
	cmd.Stderr = &output
	cmd.Stdout = &output

	if err := cmd.Run(); err != nil {
		//os.RemoveAll(tmpDir)
		log.Printf("Unexpected error: %v\n%v", err, output.String())
		return nil, err
	}
	return starlark.None, nil
}
