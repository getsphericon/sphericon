package jobs_test

import (
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func prunePasskeyChallenges(t *testing.T, env *testhelper.TestEnv, now time.Time) {
	t.Helper()
	w := jobs.NewPrunePasskeyChallengesWorker(env.DB, func() time.Time { return now })
	require.NoError(t, w.Work(t.Context(), &river.Job[jobs.PrunePasskeyChallengesArgs]{JobRow: &rivertype.JobRow{}}))
}

func challengeKept(t *testing.T, env *testhelper.TestEnv, id int64) bool {
	t.Helper()
	_, err := env.DB.PasskeyChallenge.Get(t.Context(), id)
	require.True(t, err == nil || ent.IsNotFound(err), "reading the challenge: %v", err)
	return err == nil
}

func TestPruneDeletesSpentPasskeyChallengesOlderThanTheRetention(t *testing.T) {
	env := testhelper.Setup(t)

	prunePasskeyChallenges(t, env, time.Now())

	assert.False(t, challengeKept(t, env, fixtures.SpentPasskeyChallengeOldID))
}

func TestPruneKeepsSpentPasskeyChallengesInsideTheRetention(t *testing.T) {
	env := testhelper.Setup(t)

	prunePasskeyChallenges(t, env, time.Now())

	assert.True(t, challengeKept(t, env, fixtures.SpentPasskeyChallengeYoungID), "a challenge a cookie could still carry is kept")
}
