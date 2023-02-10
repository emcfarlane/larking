package starlarkos

import (
	"bytes"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"

	"go.starlark.net/starlark"
	"larking.io/starlib/starext"
	"larking.io/starlib/starlarkstruct"
	"larking.io/starlib/starlarkthread"
)

func NewModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "os",
		Members: starlark.StringDict{
			"exec": starext.MakeBuiltin("os.exec", Exec),
		},
	}
}

// Exec runs external commands.
func Exec(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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

	ctx := starlarkthread.GetContext(thread)
	cmd := exec.CommandContext(ctx, name, cmdArgs...)
	cmd.Dir = filepath.ToSlash(dir)
	cmd.Env = cmdEnv

	var output bytes.Buffer
	cmd.Stderr = &output
	cmd.Stdout = &output

	if err := cmd.Run(); err != nil {
		//os.RemoveAll(tmpDir)
		log.Printf("Unexpected error: %v\n%v", err, output.String())
		return nil, err
	}
	return starlark.String(output.String()), nil
}
