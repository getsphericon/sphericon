package narrow_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/narrow"
)

func TestInt32KeepsWhatFitsAndCapsWhatDoesNot(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.Int64().Draw(t, "n")

		got := narrow.Int32(n)

		want := min(max(n, math.MinInt32), math.MaxInt32)
		assert.Equal(t, want, int64(got))
	})
}
