package fixtures_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
)

// A fixture time is an instant, not a wall-clock reading in the database session's
// zone: it carries its UTC offset, so a server running in another zone stores the same
// moment (the analytics series buckets by UTC day).
func TestRelativeFixtureTimesCarryAnExplicitUTCOffset(t *testing.T) {
	funcs := fixtures.TemplateFuncs(nil, time.Now())
	daysAgo, ok := funcs["daysAgo"].(func(int) string)
	require.True(t, ok)

	got, err := time.Parse("2006-01-02 15:04:05-07", daysAgo(3))
	require.NoError(t, err)

	assert.WithinDuration(t, time.Now().Add(-72*time.Hour), got, time.Minute)
	_, offset := got.Zone()
	assert.Zero(t, offset)
}
