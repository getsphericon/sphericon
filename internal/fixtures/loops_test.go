package fixtures_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
)

// A fixture repeats a row with {{range $i := seq from to}}: both ends are included.
func TestSeqCountsFromOneEndToTheOtherInclusive(t *testing.T) {
	funcs := fixtures.TemplateFuncs(nil, time.Now())
	seq, ok := funcs["seq"].(func(int, int) []int)
	require.True(t, ok)

	assert.Equal(t, []int{5, 6, 7}, seq(5, 7))
}

// A fixture time ahead of now is an instant too: it carries the UTC offset.
func TestHoursAheadIsInTheFutureWithAnExplicitUTCOffset(t *testing.T) {
	funcs := fixtures.TemplateFuncs(nil, time.Now())
	hoursAhead, ok := funcs["hoursAhead"].(func(int) string)
	require.True(t, ok)

	got, err := time.Parse("2006-01-02 15:04:05-07", hoursAhead(2))
	require.NoError(t, err)

	assert.WithinDuration(t, time.Now().Add(2*time.Hour), got, time.Minute)
}

// monthAgo pins a fixture to a calendar month: the 3rd at 10:00 of the month two months back.
func TestMonthAgoIsAMomentInsideAnEarlierCalendarMonth(t *testing.T) {
	now := time.Date(2026, time.March, 31, 15, 0, 0, 0, time.UTC)
	funcs := fixtures.TemplateFuncs(nil, now)
	monthAgo, ok := funcs["monthAgo"].(func(int, int, int, int) string)
	require.True(t, ok)

	got, err := time.Parse("2006-01-02 15:04:05-07", monthAgo(2, 3, 10, 0))
	require.NoError(t, err)

	assert.Equal(t, time.Date(2026, time.January, 3, 10, 0, 0, 0, time.UTC), got.UTC())
}
