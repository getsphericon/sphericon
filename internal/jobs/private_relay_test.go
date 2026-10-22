package jobs_test

import (
	"errors"
	"testing"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/privaterelayrange"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The refresh job replaces the stored Private Relay ranges with the published list, and a failed
// fetch fails the job (river retries it) and leaves the stored ranges as they were (ADR 0053).
func TestRefreshPrivateRelayJobReplacesTheRangesAndKeepsThemOnFailure(t *testing.T) {
	env := testhelper.Setup(t)
	w := jobs.NewRefreshPrivateRelayWorker(env.Machine)
	job := &river.Job[jobs.RefreshPrivateRelayArgs]{JobRow: &rivertype.JobRow{}}
	stored := func() []string {
		t.Helper()
		got, err := env.DB.PrivateRelayRange.Query().Select(privaterelayrange.FieldCidr).Strings(t.Context())
		require.NoError(t, err)
		return got
	}
	before := stored()
	require.NotEmpty(t, before)

	env.PrivateRelay.Fail(errors.New("apple is down"))
	require.Error(t, w.Work(t.Context(), job))
	assert.ElementsMatch(t, before, stored())

	env.PrivateRelay.Publish("203.0.113.0/24,US,US-CA,Los Angeles,\n2001:db8:2::/48,US,US-CA,Los Angeles,\n203.0.113.0/24,US,US-CA,Los Angeles,\n")
	require.NoError(t, w.Work(t.Context(), job))
	assert.ElementsMatch(t, []string{"203.0.113.0/24", "2001:db8:2::/48"}, stored())
}
