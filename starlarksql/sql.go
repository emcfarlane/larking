// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sql provides an interface to conntect to SQL databases.
package starlarksql

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/emcfarlane/larking/starext"
	"github.com/emcfarlane/larking/starlarkerrors"
	"github.com/emcfarlane/larking/starlarkthread"
	starlarktime "go.starlark.net/lib/time"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"gocloud.dev/mysql"
	"gocloud.dev/postgres"
)

func NewModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "sql",
		Members: starlark.StringDict{
			"open": starext.MakeBuiltin("sql.open", Open),

			// sql errors
			"err_conn_done": starlarkerrors.NewError(sql.ErrConnDone),
			"err_no_rows":   starlarkerrors.NewError(sql.ErrNoRows),
			"err_tx_done":   starlarkerrors.NewError(sql.ErrTxDone),
		},
	}
}

// genQueryOptions generates standard query options.
func genQueryOptions(q url.Values) string {
	if s := q.Encode(); s != "" {
		return "?" + s
	}
	return ""
}

// genOpaque generates a opaque file path DSN from the passed URL.
func genOpaque(u *url.URL) (string, error) {
	if u.Opaque == "" {
		return "", fmt.Errorf("error missing path")
	}
	return u.Opaque + genQueryOptions(u.Query()), nil
}

func Open(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 1, &name); err != nil {
		return nil, err
	}

	u, err := url.Parse(name)
	if err != nil {
		return nil, err
	}

	ctx := starlarkthread.Context(thread)

	var db *sql.DB
	switch {
	case strings.HasSuffix(u.Scheme, "mysql"):
		db, err = mysql.Open(ctx, name)
	case strings.HasSuffix(u.Scheme, "postgres"):
		db, err = postgres.Open(ctx, name)
	case u.Scheme == "sqlite":
		// build dsn
		dsn, derr := genOpaque(u)
		if derr != nil {
			return nil, derr
		}

		db, err = sql.Open("sqlite", dsn)

	default:
		return nil, fmt.Errorf("unsupported database %s", u.Scheme)
	}
	if err != nil {
		return nil, err
	}

	v := NewDB(name, db)
	if err := starlarkthread.AddResource(thread, v); err != nil {
		return nil, err
	}
	return v, nil
}

type DB struct {
	name string
	db   *sql.DB

	frozen bool
}

func NewDB(name string, db *sql.DB) *DB { return &DB{name: name, db: db} }
func (db *DB) Close() error             { return db.db.Close() }

func (v *DB) String() string        { return fmt.Sprintf("<db %q>", v.name) }
func (v *DB) Type() string          { return "sql.db" }
func (v *DB) Freeze()               { v.frozen = true } // immutable?
func (v *DB) Truth() starlark.Bool  { return v.db != nil }
func (v *DB) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: %s", v.Type()) }

type dbAttr func(v *DB) starlark.Value

var dbAttrs = map[string]dbAttr{
	"exec":      func(v *DB) starlark.Value { return starext.MakeMethod(v, "exec", v.exec) },
	"query":     func(v *DB) starlark.Value { return starext.MakeMethod(v, "query", v.query) },
	"query_row": func(v *DB) starlark.Value { return starext.MakeMethod(v, "query_row", v.queryRow) },
	"ping":      func(v *DB) starlark.Value { return starext.MakeMethod(v, "ping", v.ping) },
	"close":     func(v *DB) starlark.Value { return starext.MakeMethod(v, "close", v.close) },
}

func (v *DB) Attr(name string) (starlark.Value, error) {
	if a := dbAttrs[name]; a != nil {
		return a(v), nil
	}
	return nil, nil
}
func (v *DB) AttrNames() []string {
	names := make([]string, 0, len(dbAttrs))
	for name := range dbAttrs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

type Result struct {
	result sql.Result
}

func (r *Result) String() string        { return fmt.Sprintf("<result %t>", r.result != nil) }
func (r *Result) Type() string          { return "sql.result" }
func (r *Result) Freeze()               {} // immutable
func (r *Result) Truth() starlark.Bool  { return r.result != nil }
func (r *Result) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: %s", r.Type()) }
func (r *Result) AttrNames() []string   { return []string{"last_insert_id", "rows_affected"} }
func (r *Result) Attr(name string) (starlark.Value, error) {
	switch name {
	case "last_insert_id":
		i, err := r.result.LastInsertId()
		if err != nil {
			return nil, err
		}
		return starlark.MakeInt64(i), nil
	case "rows_affected":
		i, err := r.result.RowsAffected()
		if err != nil {
			return nil, err
		}
		return starlark.MakeInt64(i), nil
	default:
		return nil, nil
	}
}

func makeArgs(args starlark.Tuple) ([]interface{}, error) {
	// translate arg types
	xs := make([]interface{}, len(args))
	for i, arg := range args {
		switch arg := arg.(type) {
		case starlark.NoneType:
			xs[i] = nil
		case starlark.Bool:
			xs[i] = bool(arg)
		case starlark.String:
			xs[i] = string(arg)
		case starlark.Bytes:
			xs[i] = []byte(arg)
		case starlark.Int:
			x, ok := arg.Uint64()
			if !ok {
				return nil, fmt.Errorf("invalid arg int too larg: %v", arg.String())
			}
			xs[i] = x
		case starlark.Float:
			xs[i] = float64(arg)
		// TODO: time
		case driver.Valuer:
			x, err := arg.Value()
			if err != nil {
				return nil, err
			}
			xs[i] = x
		default:
			return nil, fmt.Errorf("invalid arg type: %v", arg.Type())
		}
	}
	return xs, nil
}

//func dbBeginTx(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
//	return nil, nil // TODO: Create struct TX.
//}

func (v *DB) exec(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	queryArgs := args
	if len(args) > 1 {
		queryArgs = args[:1]
	}
	var query string
	if err := starlark.UnpackPositionalArgs(fnname, queryArgs, kwargs, 1, &query); err != nil {
		return nil, err
	}

	dbArgs, err := makeArgs(args[1:])
	if err != nil {
		return nil, err
	}

	ctx := starlarkthread.Context(thread)
	result, err := v.db.ExecContext(ctx, query, dbArgs...)
	if err != nil {
		return nil, err
	}
	return &Result{result: result}, nil

}

func (v *DB) query(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	queryArgs := args
	if len(args) > 1 {
		queryArgs = args[:1]
	}
	var query string
	if err := starlark.UnpackPositionalArgs(fnname, queryArgs, kwargs, 1, &query); err != nil {
		return nil, err
	}

	dbArgs, err := makeArgs(args[1:])
	if err != nil {
		return nil, err
	}

	ctx := starlarkthread.Context(thread)
	rows, err := v.db.QueryContext(ctx, query, dbArgs...)
	if err != nil {
		return nil, err
	}

	cols, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	columns := make([]string, len(cols))
	mapping := make(map[string]int, len(cols))
	for i, col := range cols {
		columns[i] = col.Name()
		mapping[col.Name()] = i
	}

	r := &Rows{
		columns: columns,
		mapping: mapping,
		rows:    rows,
	}
	if err := starlarkthread.AddResource(thread, r); err != nil {
		return nil, err
	}
	return r, nil
}

func (v *DB) queryRow(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	queryArgs := args
	if len(args) > 1 {
		queryArgs = args[:1]
	}
	var query string
	if err := starlark.UnpackPositionalArgs(fnname, queryArgs, kwargs, 1, &query); err != nil {
		return nil, err
	}

	dbArgs, err := makeArgs(args[1:])
	if err != nil {
		return nil, err
	}

	ctx := starlarkthread.Context(thread)
	rows, err := v.db.QueryContext(ctx, query, dbArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	columns := make([]string, len(cols))
	for i, col := range cols {
		columns[i] = col.Name()
	}

	if !rows.Next() {
		return nil, sql.ErrNoRows
	}

	m := make(map[string]int, len(columns))
	x := &Row{
		mapping: m,
		values:  make([]starlark.Value, len(columns)),
	}

	dest := make([]interface{}, len(columns))
	for i, name := range columns {
		m[name] = i
		dest[i] = x.scanAt(i)
	}

	if err := rows.Scan(dest...); err != nil {
		return nil, err
	}
	return x, nil
}

func (v *DB) ping(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackPositionalArgs(fnname, args, kwargs, 0); err != nil {
		return nil, err
	}

	ctx := starlarkthread.Context(thread)
	if err := v.db.PingContext(ctx); err != nil {
		return nil, err
	}
	return starlark.None, nil
}

func (v *DB) close(_ *starlark.Thread, fnname string, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if err := v.db.Close(); err != nil {
		return nil, err
	}
	return starlark.None, nil
}

type Rows struct {
	columns []string
	mapping map[string]int
	rows    *sql.Rows

	frozen   bool
	iterErr  error
	closeErr error
}

func (v *Rows) Close() error {
	v.Freeze()
	return v.closeErr
}
func (v *Rows) String() string { return fmt.Sprintf("<rows %s>", strings.Join(v.columns, ", ")) }
func (v *Rows) Type() string   { return "sql.rows" }
func (v *Rows) Freeze() {
	if !v.frozen {
		v.closeErr = v.rows.Close()
	}
	v.frozen = true
}
func (v *Rows) Truth() starlark.Bool  { return v.rows != nil }
func (v *Rows) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: %s", v.Type()) }

func (v *Rows) Iterate() starlark.Iterator {
	return v
}

func (v *Rows) Next(p *starlark.Value) bool {
	if ok := v.rows.Next(); !ok {
		return false
	}

	x := &Row{
		mapping: v.mapping,
		values:  make([]starlark.Value, len(v.columns)),
	}

	dest := make([]interface{}, len(v.columns))
	for i := range v.columns {
		dest[i] = x.scanAt(i)
	}

	v.iterErr = v.rows.Scan(dest...)
	*p = x
	return v.iterErr == nil
}
func (v *Rows) Done() {
	v.closeErr = v.rows.Close()
	v.frozen = true
}

type Row struct {
	mapping map[string]int
	values  []starlark.Value
}

func (v *Row) String() string        { return fmt.Sprintf("<row %q>", strings.Join(v.AttrNames(), ", ")) }
func (v *Row) Type() string          { return "sql.row" }
func (v *Row) Freeze()               {} // immutable
func (v *Row) Truth() starlark.Bool  { return len(v.values) > 0 }
func (v *Row) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: %s", v.Type()) }

func (v *Row) Attr(name string) (starlark.Value, error) {
	if i, ok := v.mapping[name]; ok {
		return v.values[i], nil
	}
	return nil, fmt.Errorf("unknown name")
}
func (v *Row) AttrNames() []string {
	names := make([]string, 0, len(v.mapping))
	for name := range v.mapping {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
func (v *Row) Index(i int) starlark.Value { return v.values[i] }
func (v *Row) Len() int                   { return len(v.mapping) }

type scanFn func(value interface{}) error

func (f scanFn) Scan(value interface{}) error { return f(value) }

func (r *Row) scanAt(index int) scanFn {
	return func(value interface{}) (err error) {
		var v starlark.Value
		switch x := value.(type) {
		case int64:
			v = starlark.MakeInt64(x)
		case float64:
			v = starlark.Float(x)
		case bool:
			v = starlark.Bool(x)
		case []byte:
			v = starlark.Bytes(string(x))
		case string:
			v = starlark.String(x)
		case time.Time:
			v = starlarktime.Time(x)
		case nil:
			v = starlark.None
		default:
			return fmt.Errorf("unhandled type: %T", value)
		}
		r.values[index] = v
		return
	}
}
