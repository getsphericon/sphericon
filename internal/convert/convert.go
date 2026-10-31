// Package convert holds small generic adapters between the ogen-generated
// Opt*/OptNil* option types and the plain Go pointers that ent setters expect.
// They replace the per-package optNil* helpers that the site and external API
// handlers previously duplicated.
package convert

import (
	"encoding/json"
	"time"

	"github.com/go-faster/jx"

	"github.com/getsphericon/sphericon/internal/patch"
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

// option is implemented by every ogen Opt*/OptNil* value type: IsSet reports whether the key was
// present, and Get whether it carried a value (an OptNil's explicit null is present, without one).
type option[T any] interface {
	getter[T]
	IsSet() bool
}

// Patch is a request field as a patch field (JSON Merge Patch): an absent key keeps the stored
// value, an explicit null clears it, a value sets it. An Opt that cannot be null never clears.
func Patch[T any, O option[T]](o O) patch.Field[T] {
	v, ok := o.Get()
	return patch.Of(v, o.IsSet(), o.IsSet() && !ok)
}

// StringPatch is Patch for a named string (an EmailAddress, a TimeZoneName), as a plain string.
func StringPatch[T ~string, O option[T]](o O) patch.Field[string] {
	return patch.Map(Patch(o), func(v T) string { return string(v) })
}

// Int64Patch is Patch for a named int64 (an EntityId), as a plain int64.
func Int64Patch[T ~int64, O option[T]](o O) patch.Field[int64] {
	return patch.Map(Patch(o), func(v T) int64 { return int64(v) })
}

// IntPatch is Patch for a named int32 (a limit), as an int.
func IntPatch[T ~int32, O option[T]](o O) patch.Field[int] {
	return patch.Map(Patch(o), func(v T) int { return int(v) })
}

// ListPatch is a list field that cannot be null as a patch field: an absent key (a nil slice)
// keeps the stored list, any list (empty included) replaces it.
func ListPatch[S ~[]E, E any](list S) patch.Field[[]E] {
	if list == nil {
		return patch.Keep[[]E]()
	}
	return patch.Set([]E(list))
}

// StringsPatch is ListPatch for a list of named strings, as plain strings.
func StringsPatch[S ~[]E, E ~string](list S) patch.Field[[]string] {
	if list == nil {
		return patch.Keep[[]string]()
	}
	out := make([]string, len(list))
	for i, v := range list {
		out[i] = string(v)
	}
	return patch.Set(out)
}

// DocPatch is an opaque editor-document field as a patch field of the JSON the modules render: an
// absent key keeps the stored document.
func DocPatch[M ~map[string]jx.Raw, O option[M]](o O) patch.Field[json.RawMessage] {
	v, ok := o.Get()
	if !ok {
		return patch.Keep[json.RawMessage]()
	}
	return patch.Set(DocJSON(v, true))
}

// MapPatch is an opaque object field (a Record of raw JSON values) as a patch field of plain values:
// an absent key keeps, an explicit null clears.
func MapPatch[M ~map[string]jx.Raw, O option[M]](o O) patch.Field[map[string]any] {
	return patch.Map(Patch(o), RawMap[M])
}

// DocJSON is the opaque editor-document object of a write as the JSON the modules render; nil when
// the request names none. An empty object is a document (an invalid one), not an absent one.
func DocJSON[M ~map[string]jx.Raw](fields M, set bool) json.RawMessage {
	if !set {
		return nil
	}
	raw := make(map[string]json.RawMessage, len(fields))
	for k, v := range fields {
		raw[k] = json.RawMessage(v)
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	return b
}

// DocObject is a stored editor document as the opaque object of a contract; nil for a message
// written as HTML, which has none.
func DocObject[M ~map[string]jx.Raw](doc json.RawMessage) M {
	if len(doc) == 0 {
		return nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(doc, &fields); err != nil {
		return nil
	}
	out := make(M, len(fields))
	for k, v := range fields {
		out[k] = jx.Raw(v)
	}
	return out
}

// Nil is the ogen Nil* value N of v (NilInt64, NilCurrencyCode, ...): null when v is nil. Every
// surface's package has its own N, so its mapper names the type and the rule stays here.
func Nil[N any, T any, P interface {
	*N
	SetTo(v T)
	SetToNull()
}](v *T) N {
	var n N
	if v == nil {
		P(&n).SetToNull()
	} else {
		P(&n).SetTo(*v)
	}
	return n
}

// WholeSeconds is d in whole seconds, nil when there is no duration.
func WholeSeconds(d *time.Duration) *int64 {
	if d == nil {
		return nil
	}
	seconds := int64(d.Seconds())
	return &seconds
}
