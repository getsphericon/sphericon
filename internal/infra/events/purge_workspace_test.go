package events_test

import (
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// workspacesOfJobs is the Workspace each queued job of kind names, oldest first; 0 for a job that
// names none.
func workspacesOfJobs(t *testing.T, env *testhelper.TestEnv, kind string) []int64 {
	t.Helper()
	return lo.Map(env.JobsOf(t, kind), func(args map[string]any, _ int) int64 {
		id, _ := args[jobkind.WorkspaceArg].(float64)
		return int64(id)
	})
}

// The purge of a Workspace's jobs removes every job that names it, whatever its kind, queue or
// state (waiting, scheduled, discarded), and leaves a running one to finish, every job of another
// Workspace and the jobs of the Instance.
func TestPurgeWorkspaceJobsRemovesEveryJobOfTheWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	later := time.Now().Add(7 * 24 * time.Hour)
	for _, ws := range []int64{fixtures.InitechID, fixtures.AcmeID} {
		env.EnqueueJob(t, suppressJob(t, ws, 0, "bounced@example.com", "bounce"))
		env.EnqueueDiscardedJob(t, jobs.DeliverWebhookArgs{WorkspaceID: ws, EndpointID: 1, EventName: "x", DeliveryID: "d", Body: []byte(`{"data":{"email":"bounced@example.com"}}`)})
		env.EnqueueScheduledJob(t, broadcasts.SendArgs{WorkspaceID: ws, BroadcastID: 1}, later)
	}
	env.EnqueueRunningJob(t, automations.RunStepArgs{WorkspaceID: fixtures.InitechID, RunID: 1})
	env.EnqueueJob(t, jobs.PruneEventsArgs{})

	require.NoError(t, env.Bus.WithinWorkspacePurgeTx(ctx, func(_ *ent.Client, purger events.WorkspacePurger) error {
		return purger.PurgeWorkspaceJobs(ctx, fixtures.InitechID)
	}))

	suppress := workspacesOfJobs(t, env, jobs.SuppressEventArgs{}.Kind())
	deliveries := workspacesOfJobs(t, env, jobs.DeliverWebhookArgs{}.Kind())
	sends := workspacesOfJobs(t, env, broadcasts.SendArgs{}.Kind())
	steps := workspacesOfJobs(t, env, automations.RunStepArgs{}.Kind())
	prunes := env.JobsOf(t, jobs.PruneEventsArgs{}.Kind())
	assert.Equal(t, []int64{fixtures.AcmeID}, suppress, "a waiting event job goes")
	assert.Equal(t, []int64{fixtures.AcmeID}, deliveries, "a discarded delivery goes")
	assert.Equal(t, []int64{fixtures.AcmeID}, sends, "a scheduled send goes")
	assert.Equal(t, []int64{fixtures.InitechID}, steps, "a running job is left to finish")
	assert.Len(t, prunes, 1, "a job of the Instance stays")
}

// A delivery the webhooks consumer queues names its Workspace, so the purge finds the body a
// Webhook endpoint of the Workspace would have received.
func TestPurgeWorkspaceJobsRemovesTheDeliveriesTheWebhooksConsumerQueued(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	deliveryBody(t, env, &events.ContactCreated{WorkspaceID: fixtures.InitechID, ContactID: fixtures.ContactErasableID})
	queued := workspacesOfJobs(t, env, jobs.DeliverWebhookArgs{}.Kind())
	require.Equal(t, []int64{fixtures.InitechID}, queued, "the delivery names its Workspace")

	require.NoError(t, env.Bus.WithinWorkspacePurgeTx(ctx, func(_ *ent.Client, purger events.WorkspacePurger) error {
		return purger.PurgeWorkspaceJobs(ctx, fixtures.InitechID)
	}))

	assert.Empty(t, env.JobsOf(t, jobs.DeliverWebhookArgs{}.Kind()))
}
