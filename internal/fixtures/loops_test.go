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
