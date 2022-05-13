package starlarkrule

import (
	"fmt"

	"github.com/emcfarlane/larking/starlib/starlarkstruct"
	"go.starlark.net/starlark"
)

// toStruct converts the value and checks the constructor type matches.
func toStruct(v starlark.Value, constructor starlark.Value) (*starlarkstruct.Struct, error) {
	if v == nil {
		return nil, fmt.Errorf("missing struct value")
	}
	s, ok := v.(*starlarkstruct.Struct)
	if !ok {
		return nil, fmt.Errorf("invalid type: %T, expected struct", v)
	}
	return s, assertConstructor(s, constructor)
}

// Constructor values must be comparable.
func assertConstructor(s *starlarkstruct.Struct, c starlark.Value) error {
	if sc := s.Constructor(); sc != c {
		return fmt.Errorf("invalid struct type: %s, want %s", sc, c)
	}
	return nil
}

func getAttrStr(v *starlarkstruct.Struct, name string) (string, error) {
	x, err := v.Attr(name)
	if err != nil {
		return "", err
	}
	s, ok := starlark.AsString(x)
	if !ok {
		return "", fmt.Errorf("attr %q not a string", name)
	}
	return s, nil
}
