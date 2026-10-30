package jobs_test

import (
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func prunePendingSignups(t *testing.T, env *testhelper.TestEnv, now time.Time) {
	t.Helper()
	w := jobs.NewPrunePendingSignupsWorker(accounts.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords), func() time.Time { return now })
	require.NoError(t, w.Work(t.Context(), &river.Job[jobs.PrunePendingSignupsArgs]{JobRow: &rivertype.JobRow{}}))
}

func pendingSignupKept(t *testing.T, env *testhelper.TestEnv, id int64) bool {
	t.Helper()
	_, err := env.DB.PendingSignup.Get(t.Context(), id)
	require.True(t, err == nil || ent.IsNotFound(err), "reading the pending sign-up: %v", err)
	return err == nil
}

// ADR 0062: what an abandoned provider sign-up proved is not kept.
func TestPruneDeletesExpiredPendingSignups(t *testing.T) {
	env := testhelper.Setup(t)

	prunePendingSignups(t, env, time.Now())

	assert.False(t, pendingSignupKept(t, env, fixtures.PendingSignupExpiredID))
}

func TestPruneKeepsPendingSignupsStillOpen(t *testing.T) {
	env := testhelper.Setup(t)

	prunePendingSignups(t, env, time.Now())

	assert.True(t, pendingSignupKept(t, env, fixtures.PendingSignupOpenID), "a person may still answer it")
}
