package events_test

import (
	"errors"
	"testing"

	"github.com/riverqueue/river"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A job a worker already holds is left alone (ADR 0021): the send is on its way, and the
// worker re-checks that what it refers to still exists. A queued one is removed.
func TestPurgeJobsKeepsARunningJobAndRemovesAQueuedOne(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	env.EnqueueJob(t, jobs.SendRecipientArgs{WorkspaceID: fixtures.InitechID, RecipientID: fixtures.BroadcastRecipientErasablePendingID, BroadcastID: 1})
	env.EnqueueRunningJob(t, jobs.SendRecipientArgs{WorkspaceID: fixtures.InitechID, RecipientID: fixtures.BroadcastRecipientErasableSoloID, BroadcastID: 1})

	scope := env.DB.Scoped(fixtures.InitechID)
	ids := []int64{fixtures.BroadcastRecipientErasablePendingID, fixtures.BroadcastRecipientErasableSoloID}
	require.NoError(t, env.Bus.WithinScopedPurgeTx(ctx, scope, func(_ *ent.Scoped, pub events.PurgingPublisher) error {
		return pub.PurgeJobs(ctx, fixtures.InitechID, jobkind.SendRecipient, ids)
	}))

	left := env.JobsOf(t, jobkind.SendRecipient.Kind)
	require.Len(t, left, 1)
	assert.EqualValues(t, fixtures.BroadcastRecipientErasableSoloID, left[0]["recipient_id"])
}

// river deletes a batch at a time: a purge with more queued jobs than one batch removes them
// all, not just the first batch.
func TestPurgeJobsRemovesMoreThanOneBatch(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	const queued = 1001 // one more than the purge's batch
	args := make([]river.JobArgs, queued)
	for i := range args {
		args[i] = jobs.SendRecipientArgs{WorkspaceID: fixtures.InitechID, RecipientID: fixtures.BroadcastRecipientErasablePendingID, BroadcastID: int64(i + 1)}
	}
	env.EnqueueJobs(t, args...)

	scope := env.DB.Scoped(fixtures.InitechID)
	require.NoError(t, env.Bus.WithinScopedPurgeTx(ctx, scope, func(_ *ent.Scoped, pub events.PurgingPublisher) error {
		return pub.PurgeJobs(ctx, fixtures.InitechID, jobkind.SendRecipient, []int64{fixtures.BroadcastRecipientErasablePendingID})
	}))

	assert.Empty(t, env.JobsOf(t, jobkind.SendRecipient.Kind))
}

// PurgeJobsIn purges in the Erasure's transaction the scope is bound to: the purge commits with it
// and rolls back with it.
func TestPurgeJobsInJoinsThePurgingTransactionOfTheScope(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	env.EnqueueJob(t, jobs.SendRecipientArgs{WorkspaceID: fixtures.InitechID, RecipientID: fixtures.BroadcastRecipientErasablePendingID, BroadcastID: 1})
	scope := env.DB.Scoped(fixtures.InitechID)
	ids := []int64{fixtures.BroadcastRecipientErasablePendingID}
	boom := errors.New("erasure failed")

	err := env.Bus.WithinScopedPurgeTx(ctx, scope, func(ts *ent.Scoped, _ events.PurgingPublisher) error {
		require.NoError(t, env.Bus.PurgeJobsIn(ctx, ts, jobkind.SendRecipient, ids))
		return boom
	})
	require.ErrorIs(t, err, boom)
	assert.Len(t, env.JobsOf(t, jobkind.SendRecipient.Kind), 1, "the purge rolls back with the transaction")

	require.NoError(t, env.Bus.WithinScopedPurgeTx(ctx, scope, func(ts *ent.Scoped, _ events.PurgingPublisher) error {
		return env.Bus.PurgeJobsIn(ctx, ts, jobkind.SendRecipient, ids)
	}))
	assert.Empty(t, env.JobsOf(t, jobkind.SendRecipient.Kind), "the purge commits with the transaction")
}

// Only the Erasure's transaction purges: a scope bound to another transaction of the bus is
// ErrNotPurging, one bound to none ErrNotInBusTx, and no job goes.
func TestPurgeJobsInRefusesAScopeOutsideAPurgingTransaction(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	env.EnqueueJob(t, jobs.SendRecipientArgs{WorkspaceID: fixtures.InitechID, RecipientID: fixtures.BroadcastRecipientErasablePendingID, BroadcastID: 1})
	scope := env.DB.Scoped(fixtures.InitechID)
	ids := []int64{fixtures.BroadcastRecipientErasablePendingID}

	require.ErrorIs(t, env.Bus.PurgeJobsIn(ctx, scope, jobkind.SendRecipient, ids), events.ErrNotInBusTx)
	require.NoError(t, env.Bus.WithinScopedTx(ctx, scope, func(ts *ent.Scoped, _ events.Publisher) error {
		require.ErrorIs(t, env.Bus.PurgeJobsIn(ctx, ts, jobkind.SendRecipient, ids), events.ErrNotPurging)
		return nil
	}))
	assert.Len(t, env.JobsOf(t, jobkind.SendRecipient.Kind), 1)
}

// A refusal's reach-back names the Destination that refused: the purge of a Destination's jobs
// removes the Workspace's reach-backs of the subject's Destinations, and leaves another
// Destination's and another Workspace's.
func TestPurgeDestinationJobsRemovesTheJobsOfTheSubjectsDestinations(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	env.EnqueueJobs(t,
		jobs.TrackingWithdrawalArgs{WorkspaceID: fixtures.InitechID, Channel: channels.SMS, Destination: erasablePhone},
		jobs.TrackingWithdrawalArgs{WorkspaceID: fixtures.InitechID, Channel: channels.Telegram, Destination: "801001"},
		jobs.TrackingWithdrawalArgs{WorkspaceID: fixtures.InitechID, Channel: channels.SMS, Destination: "+14155550198"},
		jobs.TrackingWithdrawalArgs{WorkspaceID: fixtures.GlobexID, Channel: channels.SMS, Destination: erasablePhone},
	)

	scope := env.DB.Scoped(fixtures.InitechID)
	require.NoError(t, env.Bus.WithinScopedPurgeTx(ctx, scope, func(_ *ent.Scoped, pub events.PurgingPublisher) error {
		return pub.PurgeDestinationJobs(ctx, fixtures.InitechID, jobkind.TrackingWithdrawal, []string{erasablePhone, "801001"})
	}))

	left := lo.Map(env.JobsOf(t, jobkind.TrackingWithdrawal.Kind), func(args map[string]any, _ int) [2]any {
		return [2]any{args["workspaceId"], args["destination"]}
	})
	assert.ElementsMatch(t, [][2]any{
		{float64(fixtures.InitechID), "+14155550198"}, {float64(fixtures.GlobexID), erasablePhone},
	}, left)
}
