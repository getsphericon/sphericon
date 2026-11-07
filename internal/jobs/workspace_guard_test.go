package jobs_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// enqueueIn queues args through the bus in a transaction of Acme, as every producer does.
func enqueueIn(t *testing.T, env *testhelper.TestEnv, args broadcasts.SendArgs) error {
	t.Helper()
	return env.Bus.WithinScopedEnqueueTx(t.Context(), env.DB.Scoped(fixtures.AcmeID), func(_ *ent.Scoped, _ events.Publisher, q events.Enqueuer) error {
		return q.Enqueue(t.Context(), args, nil)
	})
}

// A job of a Workspace queued without its Workspace is refused, so none waits where the erase of
// its Workspace would never find it; one that names it is queued.
func TestAJobOfAWorkspaceIsQueuedOnlyWithItsWorkspace(t *testing.T) {
	env := testhelper.Setup(t)

	unnamed := enqueueIn(t, env, broadcasts.SendArgs{BroadcastID: fixtures.BroadcastDraftID})
	named := enqueueIn(t, env, broadcasts.SendArgs{WorkspaceID: fixtures.AcmeID, BroadcastID: fixtures.BroadcastDraftID})

	require.ErrorIs(t, unnamed, jobs.ErrNoWorkspace)
	require.NoError(t, named)
	queued := env.JobsOf(t, broadcasts.SendArgs{}.Kind())
	assert.Len(t, queued, 1)
}
