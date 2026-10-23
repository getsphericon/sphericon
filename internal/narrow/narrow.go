// Package narrow converts Go's int and int64 to the int32 the API contracts use for counts and
// page numbers. The conversion saturates at the int32 bounds, so a value too large for the wire
// is capped rather than wrapped into a negative number.
package narrow

import "math"

// Int32 is n as an int32, capped at math.MinInt32 and math.MaxInt32.
func Int32[T ~int | ~int64](n T) int32 {
	if n > math.MaxInt32 {
		return math.MaxInt32
	}
	if n < math.MinInt32 {
		return math.MinInt32
	}
	return int32(n)
}

// Uint32 is n as a uint32, capped at 0 and math.MaxUint32.
func Uint32[T ~int | ~int64](n T) uint32 {
	if n < 0 {
		return 0
	}
	if int64(n) > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(n)
}
