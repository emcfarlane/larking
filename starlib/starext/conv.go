package starext

import (
	"fmt"
	"sort"

	"go.starlark.net/starlark"
)

type Key interface {
	string | int | float64
}

type Value interface {
	string | int | float64 | bool
}

func toValue(v any) starlark.Value {
	switch t := v.(type) {
	case string:
		return starlark.String(t)
	case int:
		return starlark.MakeInt(t)
	case float64:
		return starlark.Float(t)
	case bool:
		return starlark.Bool(t)
	default:
		panic(fmt.Errorf("unhandled type %T", v))
	}
}

func ToDict[K Key, V Value](v map[K]V) *starlark.Dict {
	// Sort keys, create dict.

	keys := make([]K, 0, len(v))
	for key := range v {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	d := starlark.NewDict(len(v))
	for _, key := range keys {
		d.SetKey(toValue(key), toValue(v[key])) //nolint
	}
	return d
}
