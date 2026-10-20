package events_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobkind"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A job a worker already holds is left alone (ADR 0021): the send is on its way, and the
// worker re-checks that what it refers to still exists. A queued one is removed.
func TestPurgeJobsKeepsARunningJobAndRemovesAQueuedOne(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	env.EnqueueJob(t, jobs.SendRecipientArgs{RecipientID: fixtures.BroadcastRecipientErasablePendingID, BroadcastID: 1})
	env.EnqueueRunningJob(t, jobs.SendRecipientArgs{RecipientID: fixtures.BroadcastRecipientErasableSoloID, BroadcastID: 1})

	scope := env.DB.Scoped(fixtures.InitechID)
	ids := []int64{fixtures.BroadcastRecipientErasablePendingID, fixtures.BroadcastRecipientErasableSoloID}
	require.NoError(t, env.Bus.WithinScopedPurgeTx(ctx, scope, func(_ *ent.Scoped, pub events.PurgingPublisher) error {
		return pub.PurgeJobs(ctx, jobkind.SendRecipient, ids)
	}))

	left := env.JobsOf(t, jobkind.SendRecipient.Kind)
	require.Len(t, left, 1)
	assert.EqualValues(t, fixtures.BroadcastRecipientErasableSoloID, left[0]["recipient_id"])
}
