package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/workspacedeletion"
)

// The job is a trigger: at its clock it erases a Workspace whose window has passed.
func TestErasureJobTriggersTheModuleAtItsClock(t *testing.T) {
	env := testhelper.Setup(t)
	now := time.Date(2026, time.October, 19, 12, 0, 0, 0, time.UTC)
	env.DB.Workspace.UpdateOneID(fixtures.UmbrellaID).
		SetDeletedAt(now.Add(-workspacedeletion.DeletionWindow - time.Hour)).ExecX(t.Context())
	w := jobs.NewEraseWorkspacesWorker(workspacedeletion.New(env.DB, env.Bus), func() time.Time { return now })

	require.NoError(t, w.Work(context.Background(), &river.Job[jobs.EraseWorkspacesArgs]{JobRow: &rivertype.JobRow{}}))

	ws := env.DB.Workspace.GetX(t.Context(), fixtures.UmbrellaID)
	require.NotNil(t, ws.ErasedAt)
	assert.True(t, now.Equal(*ws.ErasedAt))
}
