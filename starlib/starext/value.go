package starext

import (
	"reflect"

	"go.starlark.net/starlark"
)

// A Value is a Starlark value that wraps a Go value using reflection.
type Value interface {
	starlark.Value
	Reflect() reflect.Value
}
