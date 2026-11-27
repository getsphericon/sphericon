package jobs_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/sending/frequencycap"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// countedSince is how many places the count of the Acme address holds after since.
func countedSince(t *testing.T, env *testhelper.TestEnv, address string, since time.Time) int {
	t.Helper()
	n, err := testhelper.Module[*frequencycap.Module](env).Counted(t.Context(), env.DB.Scoped(fixtures.AcmeID),
		frequencycap.CountedInput{Channel: channels.Email, Destination: address, Since: since})
	require.NoError(t, err)
	return n
}

// The count is kept seven days, the longest period a cap may have (ADR 0178). Fixtures:
// FrequencyCountExpired was counted eight days ago, FrequencyCountRecent an hour ago.
func TestPruneFrequencyCountsDeletesTheRowsOlderThanSevenDays(t *testing.T) {
	env := testhelper.Setup(t)
	monthAgo := time.Now().AddDate(0, -1, 0)
	w := jobs.NewPruneFrequencyCountsWorker(testhelper.Module[*frequencycap.Module](env), env.DB)

	require.NoError(t, w.Work(t.Context(), job(jobs.PruneFrequencyCountsArgs{})))

	assert.Zero(t, countedSince(t, env, fixtures.FrequencyCountExpiredDestination, monthAgo), "the row counted eight days ago goes")
	assert.Equal(t, 1, countedSince(t, env, fixtures.FrequencyCountRecentDestination, monthAgo), "the row counted an hour ago stays")
}
