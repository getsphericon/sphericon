package deferralcause_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/kernel/deferralcause"
)

// Counts names every cause of the closed list once, in its order, with the count it was given or
// zero.
func TestCountsNamesEveryCauseWithItsCount(t *testing.T) {
	causes := lo.Map(deferralcause.Cause("").Values(), func(v string, _ int) deferralcause.Cause { return deferralcause.Cause(v) })
	rapid.Check(t, func(t *rapid.T) {
		by := rapid.MapOf(rapid.SampledFrom(causes), rapid.IntRange(1, 1000)).Draw(t, "by")

		got := deferralcause.Counts(by)

		assert.Equal(t, causes, lo.Map(got, func(c deferralcause.Count, _ int) deferralcause.Cause { return c.Cause }))
		assert.Equal(t, lo.Map(causes, func(c deferralcause.Cause, _ int) int { return by[c] }),
			lo.Map(got, func(c deferralcause.Count, _ int) int { return c.Count }))
	})
}
