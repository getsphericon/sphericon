// Package convert holds small generic adapters between the ogen-generated
// Opt*/OptNil* option types and the plain Go pointers that ent setters expect.
// They replace the per-package optNil* helpers that the site and external API
// handlers previously duplicated.
package convert

import (
	"encoding/json"

	"github.com/go-faster/jx"
)

// RawMap decodes an ogen Record<unknown> body (map of raw JSON values) into a plain
// map[string]any so value types match what the service layer expects (float64 for
// numbers, bool, nested maps, …). Used for typed custom field values on the wire.
func RawMap[M ~map[string]jx.Raw](m M) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		var decoded any
		if err := json.Unmarshal(v, &decoded); err != nil {
			continue
		}
		out[k] = decoded
	}
	return out
}

// getter is implemented by every ogen Opt*/OptNil* value type.
type getter[T any] interface {
	Get() (T, bool)
}

// Ptr returns a pointer to the option's value, or nil when it is unset/null.
func Ptr[T any, O getter[T]](o O) *T {
	if v, ok := o.Get(); ok {
		return &v
	}
	return nil
}

// Int64Ptr is like Ptr but coerces named int64 option values (e.g. an EntityId)
// down to *int64, the type ent's Nillable setters take.
func Int64Ptr[T ~int64, O getter[T]](o O) *int64 {
	if v, ok := o.Get(); ok {
		n := int64(v)
		return &n
	}
	return nil
}

// StringPtr is like Ptr but coerces named string option values (e.g. a
// TimeZoneName scalar) down to *string, the type ent's Nillable setters take.
func StringPtr[T ~string, O getter[T]](o O) *string {
	if v, ok := o.Get(); ok {
		s := string(v)
		return &s
	}
	return nil
}
