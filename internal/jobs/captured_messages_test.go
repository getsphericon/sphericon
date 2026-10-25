package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/capturedmessage"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func pruneCaptured(t *testing.T, env *testhelper.TestEnv, retention time.Duration) {
	t.Helper()
	w := jobs.NewPruneCapturedMessagesWorker(env.DB, retention)
	require.NoError(t, w.Work(context.Background(), &river.Job[jobs.PruneCapturedMessagesArgs]{JobRow: &rivertype.JobRow{}}))
}

func capturedExists(t *testing.T, env *testhelper.TestEnv, id int64) bool {
	t.Helper()
	return env.DB.CapturedMessage.Query().Where(capturedmessage.ID(id)).ExistX(context.Background())
}

func TestPruneCapturedMessagesDeletesTheExpiredAndKeepsTheRest(t *testing.T) {
	env := testhelper.Setup(t)

	pruneCaptured(t, env, 7*24*time.Hour)

	assert.False(t, capturedExists(t, env, fixtures.CapturedMessageUmbrellaExpiredOneID))
	assert.False(t, capturedExists(t, env, fixtures.CapturedMessageUmbrellaExpiredTwoID))
	assert.True(t, capturedExists(t, env, fixtures.CapturedMessageGlobexNewsletterID), "three days old, inside the window")
	assert.True(t, capturedExists(t, env, fixtures.CapturedMessageAcmeArchivedInboxID), "six days old, inside the window")
}

func TestPruneCapturedMessagesFollowsTheConfiguredWindow(t *testing.T) {
	env := testhelper.Setup(t)

	pruneCaptured(t, env, 2*24*time.Hour)

	assert.False(t, capturedExists(t, env, fixtures.CapturedMessageGlobexNewsletterID))
	assert.True(t, capturedExists(t, env, fixtures.CapturedMessageGlobexWelcomeID), "one day old")
}

func TestPruneCapturedMessagesDisabledByZeroRetention(t *testing.T) {
	env := testhelper.Setup(t)

	pruneCaptured(t, env, 0)

	assert.True(t, capturedExists(t, env, fixtures.CapturedMessageUmbrellaExpiredOneID))
}
