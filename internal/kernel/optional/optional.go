// Package optional compares the optional values a row or an input holds as pointers, nil for none.
package optional

// Equal reports whether a and b are both absent, or both present and equal.
func Equal[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// EqualFunc reports whether a and b are both absent, or both present and equal by eq.
func EqualFunc[T any](a, b *T, eq func(T, T) bool) bool {
	if a == nil || b == nil {
		return a == b
	}
	return eq(*a, *b)
}
