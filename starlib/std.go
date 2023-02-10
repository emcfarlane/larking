package starlib

import (
	"embed"
	"fmt"
	"sync"

	starlarkjson "go.starlark.net/lib/json"
	starlarkmath "go.starlark.net/lib/math"
	starlarktime "go.starlark.net/lib/time"
	"go.starlark.net/starlark"
	"larking.io/starlib/encoding/starlarkproto"
	"larking.io/starlib/net/starlarkhttp"
	"larking.io/starlib/net/starlarkopenapi"
	"larking.io/starlib/starlarkblob"
	"larking.io/starlib/starlarkdocstore"
	"larking.io/starlib/starlarkerrors"
	"larking.io/starlib/starlarkos"
	"larking.io/starlib/starlarkpubsub"
	"larking.io/starlib/starlarkrule"
	"larking.io/starlib/starlarkruntimevar"
	"larking.io/starlib/starlarksql"
	"larking.io/starlib/starlarkstruct"
	"larking.io/starlib/starlarkthread"
)

var (
	modBlob       = starlarkblob.NewModule()
	modDocstore   = starlarkdocstore.NewModule()
	modErrors     = starlarkerrors.NewModule()
	modHTTP       = starlarkhttp.NewModule() // net
	modJSON       = starlarkjson.Module      // encoding
	modMath       = starlarkmath.Module
	modOpenAPI    = starlarkopenapi.NewModule() // net
	modProto      = starlarkproto.NewModule()   // encoding
	modPubSub     = starlarkpubsub.NewModule()
	modRule       = starlarkrule.NewModule()
	modRuntimeVar = starlarkruntimevar.NewModule()
	modSQL        = starlarksql.NewModule()
	modThread     = starlarkthread.NewModule()
	modTime       = starlarktime.Module
	modOS         = starlarkos.NewModule()

	// content holds our static web server content.
	//go:embed rules/*
	local embed.FS

	stdLibMu sync.Mutex
	stdLib   = map[string]starlark.StringDict{
		"@std": makeDict(NewModule()),

		// TODO
		//"archive/container.star":           makeDict(starlarkcontainer.NewModule()),
		//"archive/tar.star":           makeDict(starlarktar.NewModule()),
		//"archive/zip.star":           makeDict(starlarkzip.NewModule()),
		//"net/grpc.star": makeDict(starlarkgrpc.NewModule()),
		"blob.star":           makeDict(modBlob),
		"docstore.star":       makeDict(modDocstore),
		"encoding/json.star":  makeDict(modJSON), // starlark
		"encoding/proto.star": makeDict(modProto),
		"errors.star":         makeDict(modErrors),
		"math.star":           makeDict(modMath), // starlark
		"net/http.star":       makeDict(modHTTP),
		"net/openapi.star":    makeDict(modOpenAPI),
		"pubsub.star":         makeDict(modPubSub),
		"runtimevar.star":     makeDict(modRuntimeVar),
		"sql.star":            makeDict(modSQL),
		"time.star":           makeDict(modTime), // starlark
		"thread.star":         makeDict(modThread),
		"rule.star":           makeDict(modRule),
		"os.star":             makeDict(modOS),
	}
)

func makeDict(module *starlarkstruct.Module) starlark.StringDict {
	dict := make(starlark.StringDict, len(module.Members)+1)
	for key, val := range module.Members {
		dict[key] = val
	}
	// Add module if no module name.
	if _, ok := dict[module.Name]; !ok {
		dict[module.Name] = module
	}
	return dict
}

func NewModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "std",
		Members: starlark.StringDict{
			"blob":       modBlob,
			"docstore":   modDocstore,
			"errors":     modErrors,
			"http":       modHTTP,
			"json":       modJSON,
			"math":       modMath,
			"openapi":    modOpenAPI,
			"os":         modOS,
			"proto":      modProto,
			"pubsub":     modPubSub,
			"rule":       modRule,
			"runtimevar": modRuntimeVar,
			"sql":        modSQL,
			"thread":     modThread,
			"time":       modTime,
		},
	}
}

// StdLoad loads files from the standard library.
func (l *Loader) StdLoad(thread *starlark.Thread, module string) (starlark.StringDict, error) {
	stdLibMu.Lock()
	if e, ok := stdLib[module]; ok {
		stdLibMu.Unlock()
		return e, nil
	}
	stdLibMu.Unlock()

	// Load and eval the file.
	src, err := local.ReadFile(module)
	if err != nil {
		return nil, err
	}
	v, err := starlark.ExecFile(thread, module, src, l.globals)
	if err != nil {
		return nil, fmt.Errorf("exec file: %v", err)
	}

	stdLibMu.Lock()
	stdLib[module] = v // cache
	stdLibMu.Unlock()
	return v, nil
}
