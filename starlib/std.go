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
	modBlob          = starlarkblob.NewModule()
	modDocstore      = starlarkdocstore.NewModule()
	modEncodingJSON  = starlarkjson.Module       // encoding
	modEncodingProto = starlarkproto.NewModule() // encoding
	modErrors        = starlarkerrors.NewModule()
	modMath          = starlarkmath.Module
	modNetHTTP       = starlarkhttp.NewModule()    // net
	modNetOpenAPI    = starlarkopenapi.NewModule() // net
	modOS            = starlarkos.NewModule()
	modPubSub        = starlarkpubsub.NewModule()
	modRule          = starlarkrule.NewModule()
	modRuntimeVar    = starlarkruntimevar.NewModule()
	modSQL           = starlarksql.NewModule()
	modThread        = starlarkthread.NewModule()
	modTime          = starlarktime.Module

	modStd = NewModule()

	// content holds our static web server content.
	//go:embed rules/*
	local embed.FS

	stdLibMu sync.Mutex
	stdLib   = map[string]starlark.StringDict{
		"std.star":  modStd.Members,
		"rule.star": modRule.Members,
	}
)

func NewModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "std",
		Members: starlark.StringDict{
			"blob":           modBlob,
			"docstore":       modDocstore,
			"encoding/json":  modEncodingJSON,
			"encoding/proto": modEncodingProto,
			"errors":         modErrors,
			"math":           modMath,
			"net/http":       modNetHTTP,
			"net/openapi":    modNetOpenAPI,
			"os":             modOS,
			"pubsub":         modPubSub,
			"runtimevar":     modRuntimeVar,
			"sql":            modSQL,
			"thread":         modThread,
			"time":           modTime,
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
