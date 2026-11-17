// Package jsonvalue compares documents as the JSON values they encode to, the way a jsonb column
// holds them (ADR 0169): key order and spacing, which jsonb does not keep, make no difference.
package jsonvalue

import (
	"encoding/json"
	"reflect"
)

// Same reports whether a and b encode to the same JSON value, whatever the order of their keys,
// a raw document's included. A value that does not round-trip through JSON is never the same: in
// doubt, a change.
func Same(a, b any) bool {
	x, okA := decoded(a)
	y, okB := decoded(b)
	return okA && okB && reflect.DeepEqual(x, y)
}

// decoded is v as the generic value its JSON decodes to, false when v does not round-trip.
func decoded(v any) (any, bool) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, false
	}
	var out any
	err = json.Unmarshal(raw, &out)
	return out, err == nil
}
