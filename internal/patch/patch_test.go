package patch_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/patch"
)

// stored is the value a column holds before a Field is laid over it: nil when it holds none.
func drawStored(t *rapid.T) *int {
	return rapid.Ptr(rapid.Int(), true).Draw(t, "stored")
}

func TestAFieldOfARequestKeepsAnAbsentKeyClearsANullAndSetsAValue(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		v := rapid.Int().Draw(t, "v")
		given := rapid.Bool().Draw(t, "given")
		null := rapid.Bool().Draw(t, "null")
		stored := drawStored(t)

		got := patch.Of(v, given, null).Over(stored)

		assert.Equal(t, expectedOver(v, given, null, stored), got)
	})
}

// expectedOver is the JSON Merge Patch rule stated plainly: absent keeps, null clears, a value sets.
func expectedOver(v int, given, null bool, stored *int) *int {
	switch {
	case !given:
		return stored
	case null:
		return nil
	default:
		return &v
	}
}

func TestApplyCallsTheSetterOnlyForASetAndTheClearerOnlyForAClear(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		v := rapid.Int().Draw(t, "v")
		f := rapid.SampledFrom([]patch.Field[int]{patch.Keep[int](), patch.Set(v), patch.Clear[int]()}).Draw(t, "field")
		stored := drawStored(t)

		got := stored
		patch.Apply(f, func(n int) bool { got = &n; return true }, func() bool { got = nil; return true })

		assert.Equal(t, f.Over(stored), got)
	})
}

func TestMapConvertsASetValueAndKeepsTheState(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		v := rapid.Int().Draw(t, "v")
		f := rapid.SampledFrom([]patch.Field[int]{patch.Keep[int](), patch.Set(v), patch.Clear[int]()}).Draw(t, "field")

		mapped := patch.Map(f, func(n int) int64 { return int64(n) * 2 })

		assert.Equal(t, f.IsKeep(), mapped.IsKeep())
		assert.Equal(t, f.IsClear(), mapped.IsClear())
		assert.Equal(t, int64PtrOf(f.Ptr()), mapped.Ptr())
	})
}

func int64PtrOf(p *int) *int64 {
	if p == nil {
		return nil
	}
	n := int64(*p) * 2
	return &n
}

func TestFromPtrSetsAValueAndKeepsNil(t *testing.T) {
	assert.Equal(t, patch.Set(3), patch.FromPtr(new(3)))
	assert.True(t, patch.FromPtr[int](nil).IsKeep())
}
