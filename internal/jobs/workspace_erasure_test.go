package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/workspacedeletion"
)

// deletionWindow is ADR 0043's window, how long a deleted Workspace stays restorable, stated here
// rather than read from the code under test.
const deletionWindow = 30 * 24 * time.Hour

// The job is a trigger: at its clock it erases a Workspace whose window has passed.
func TestErasureJobTriggersTheModuleAtItsClock(t *testing.T) {
	env := testhelper.Setup(t)
	now := time.Date(2026, time.October, 19, 12, 0, 0, 0, time.UTC)
	env.FeedDB.Workspace.UpdateOneID(fixtures.UmbrellaID).
		SetDiscardedAt(now.Add(-deletionWindow - time.Hour)).ExecX(t.Context())
	clock := func() time.Time { return now }
	w := jobs.NewEraseWorkspacesWorker(workspacedeletion.New(env.DB, env.Bus, workspaces.New(env.DB, env.Bus, clock), clock))

	require.NoError(t, w.Work(context.Background(), &river.Job[jobs.EraseWorkspacesArgs]{JobRow: &rivertype.JobRow{}}))

	ws := env.DB.Workspace.GetX(t.Context(), fixtures.UmbrellaID)
	require.NotNil(t, ws.ErasedAt)
	assert.True(t, now.Equal(*ws.ErasedAt))
}
