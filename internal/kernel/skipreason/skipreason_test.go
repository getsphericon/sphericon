package skipreason_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/kernel/skipreason"
)

// Counts names every reason of the closed list once, in its order, with the count it was given
// or zero.
func TestCountsNamesEveryReasonWithItsCount(t *testing.T) {
	reasons := lo.Map(skipreason.Reason("").Values(), func(v string, _ int) skipreason.Reason { return skipreason.Reason(v) })
	rapid.Check(t, func(t *rapid.T) {
		by := rapid.MapOf(rapid.SampledFrom(reasons), rapid.IntRange(1, 1000)).Draw(t, "by")

		got := skipreason.Counts(by)

		assert.Equal(t, reasons, lo.Map(got, func(c skipreason.Count, _ int) skipreason.Reason { return c.Reason }))
		assert.Equal(t, lo.Map(reasons, func(r skipreason.Reason, _ int) int { return by[r] }),
			lo.Map(got, func(c skipreason.Count, _ int) int { return c.Count }))
	})
}
