package broadcasts_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobkind"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// cancelFor runs CancelPending for a Contact the way Erasure does, in a purging transaction.
func cancelFor(t *testing.T, env *testhelper.TestEnv, contactID int64) error {
	t.Helper()
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.InitechID)
	return env.Bus.WithinScopedPurgeTx(ctx, s, func(ts *ent.Scoped, pub events.PurgingPublisher) error {
		return broadcasts.CancelPending(ctx, ts, contactID, pub)
	})
}

func recipientGone(env *testhelper.TestEnv, id int64) bool {
	_, err := env.DB.BroadcastRecipient.Get(context.Background(), id)
	return ent.IsNotFound(err)
}

func TestCancelPendingRemovesTheContactsUnsentRecipientsAndTheirJobs(t *testing.T) {
	env := testhelper.Setup(t)
	for _, id := range []int64{
		fixtures.BroadcastRecipientErasablePendingID, fixtures.BroadcastRecipientBystanderPendingID,
		fixtures.BroadcastRecipientErasableSoloID,
	} {
		env.EnqueueJob(t, jobs.SendRecipientArgs{RecipientID: id, BroadcastID: 1})
	}

	require.NoError(t, cancelFor(t, env, fixtures.ContactErasableID))

	assert.True(t, recipientGone(env, fixtures.BroadcastRecipientErasablePendingID))
	assert.True(t, recipientGone(env, fixtures.BroadcastRecipientErasableSoloID))
	assert.False(t, recipientGone(env, fixtures.BroadcastRecipientBystanderPendingID), "another Contact's recipient stays")
	left := env.JobsOf(t, jobkind.SendRecipient.Kind)
	require.Len(t, left, 1)
	assert.EqualValues(t, fixtures.BroadcastRecipientBystanderPendingID, left[0]["recipient_id"])
}

func TestCancelPendingKeepsARecipientWhoseSendEnded(t *testing.T) {
	env := testhelper.Setup(t)

	require.NoError(t, cancelFor(t, env, fixtures.ContactErasableID))

	assert.False(t, recipientGone(env, fixtures.BroadcastRecipientErasableID),
		"a recipient whose send ended stays as an anonymous record")
}

func TestCancelPendingSettlesABroadcastLeftWithNothingPending(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	require.NoError(t, cancelFor(t, env, fixtures.ContactErasableID))

	solo := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastInitechSendingSoloID)
	assert.Equal(t, broadcast.StatusSent, solo.Status)
	shared := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastInitechSendingID)
	assert.Equal(t, broadcast.StatusSending, shared.Status, "a pending recipient keeps the Broadcast sending")
}
