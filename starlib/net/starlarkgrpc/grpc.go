package starlarkgrpc

// TODO: create a gRPC client based off of larking.Mux.

/*import (
	"context"
	"io"
	"strings"
	"time"

	"go.starlark.net/starlark"
	"github.com/emcfarlane/larking/starlib/starlarkstruct"
	"github.com/emcfarlane/larking/starlib/starlarkthread"
	"github.com/emcfarlane/larking/starlib/starlarkproto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// Bootstrap...
// TODO: xDS

func NewModule(mux *Mux) *starlarkstruct.Module {
	s := NewStarlark(mux)
	return &starlarkstruct.Module{
		Name: "grpc",
		Members: starlark.StringDict{
			"dial":    starlark.NewBuiltin("grpc.dial", s.Dial),
		},
	}
}

type Starlark struct {
	mux *Mux
}

func NewStarlark(mux *Mux) *Starlark {
	return &Starlark{mux: mux}
}

// Dial accepts an address and optional
func (s *Starlark) Dial(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var target string
	if err := starlark.UnpackPositionalArgs("grpc.dial", args, kwargs, 1, &target); err != nil {
		return nil, err
	}

	ctx := starlarkthread.Context(thread)

	// TODO: handle security, opts.
	dialCtx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()
	cc, err := grpc.DialContext(dialCtx, target, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	if err := s.mux.RegisterConn(ctx, cc); err != nil {
		return nil, err
	}

	// TODO?
	return starlark.None, nil
}*/
