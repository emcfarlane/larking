// https://pkg.go.dev/archive/tar
package starlarktar

import (
	"database/sql"

	"go.starlark.net/starlark"
	"larking.io/starlib/starext"
	"larking.io/starlib/starlarkerrors"
	"larking.io/starlib/starlarkstruct"
)

func NewModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "tar",
		Members: starlark.StringDict{
			"open": starext.MakeBuiltin("sql.open", Open),

			// sql errors
			"err_conn_done": starlarkerrors.NewError(sql.ErrConnDone),
			"err_no_rows":   starlarkerrors.NewError(sql.ErrNoRows),
			"err_tx_done":   starlarkerrors.NewError(sql.ErrTxDone),
		},
	}
}
