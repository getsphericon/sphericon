// Package patch is one field of a partial update (JSON Merge Patch, RFC 7396): an absent key keeps
// the stored value, a value sets it and an explicit null clears it. A module's Update takes a struct
// of Fields, so a surface hands it the whole edit and cannot lose a clear on the way; the surfaces
// build a Field from their decoded request with internal/convert.
package patch

// state is what a Field asks of the stored value.
type state uint8

const (
	keeps state = iota
	sets
	clears
)

// Field is one field of an update: keep the stored value (the zero Field), set it, or clear it.
type Field[T any] struct {
	state state
	value T
}

// Keep leaves the stored value as it is: the key was absent.
func Keep[T any]() Field[T] { return Field[T]{} }

// Set replaces the stored value with v.
func Set[T any](v T) Field[T] { return Field[T]{state: sets, value: v} }

// Clear drops the stored value: the key was an explicit null.
func Clear[T any]() Field[T] { return Field[T]{state: clears} }

// Of builds a Field from a decoded request field: v is its value, given whether the key was present
// and null whether it was an explicit null. An absent key keeps, a null clears.
func Of[T any](v T, given, null bool) Field[T] {
	switch {
	case !given:
		return Keep[T]()
	case null:
		return Clear[T]()
	default:
		return Set(v)
	}
}

// FromPtr sets the value p points at, and keeps the stored one when p is nil.
func FromPtr[T any](p *T) Field[T] {
	if p == nil {
		return Keep[T]()
	}
	return Set(*p)
}

// Get returns the value a set Field carries; ok is false when it keeps or clears.
func (f Field[T]) Get() (v T, ok bool) {
	return f.value, f.state == sets
}

// Ptr returns the value a set Field carries, nil when it keeps or clears.
func (f Field[T]) Ptr() *T {
	if f.state != sets {
		return nil
	}
	v := f.value
	return &v
}

// IsKeep reports whether the Field leaves the stored value as it is.
func (f Field[T]) IsKeep() bool { return f.state == keeps }

// IsClear reports whether the Field drops the stored value.
func (f Field[T]) IsClear() bool { return f.state == clears }

// Changes reports whether the Field sets or clears the stored value.
func (f Field[T]) Changes() bool { return f.state != keeps }

// Over returns the value the Field leaves when laid over stored: stored when it keeps, nil when it
// clears, its own value when it sets.
func (f Field[T]) Over(stored *T) *T {
	switch f.state {
	case sets:
		return f.Ptr()
	case clears:
		return nil
	default:
		return stored
	}
}

// Map turns a Field of T into a Field of U, converting a set value with fn.
func Map[T, U any](f Field[T], fn func(T) U) Field[U] {
	switch f.state {
	case sets:
		return Set(fn(f.value))
	case clears:
		return Clear[U]()
	default:
		return Keep[U]()
	}
}

// Apply writes the Field into an update builder: setter for a set value, clearer for a clear, and
// nothing for a keep. The setter and clearer are an ent builder's own (upd.SetName,
// upd.ClearName), whose returned builder is ignored.
func Apply[T, B any](f Field[T], setter func(T) B, clearer func() B) {
	switch f.state {
	case sets:
		setter(f.value)
	case clears:
		clearer()
	case keeps:
	}
}

// ApplySet writes a Field of a column that cannot be cleared: setter for a set value, nothing
// otherwise. A clear of such a column is refused by the module before it builds the update.
func ApplySet[T, B any](f Field[T], setter func(T) B) {
	if v, ok := f.Get(); ok {
		setter(v)
	}
}
