package broadcasts_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/sending/linkclick"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// module is the Broadcasts module over the delivery's Outbound send.
func (d *delivery) module() *broadcasts.Module {
	return broadcasts.New(d.env.Bus, testhelper.Renderer(), d.out, linkclick.New())
}

// cancel cancels the Broadcast through the module.
func (d *delivery) cancel(id int64) (broadcasts.Cancelled, error) {
	return d.module().Cancel(d.t.Context(), d.s, id)
}

// messages counts the Outbound messages of a recipient's send.
func (d *delivery) messages(recipientID int64) int {
	n, err := d.env.DB.OutboundMessage.Query().
		Where(outboundmessage.BroadcastRecipientID(recipientID)).Count(d.t.Context())
	require.NoError(d.t, err)
	return n
}

// A sending Broadcast with one Recipient sent: Cancel ends it, and the send jobs already queued for
// the others send nothing and write nothing, on any attempt. The report counts them cancelled, with
// nothing pending.
func TestCancelStopsTheRecipientsNotSentYet(t *testing.T) {
	d := newDelivery(t, time.Now)
	alice := d.recipient(fixtures.ContactAliceID)
	carol := d.recipient(fixtures.ContactCarolID)
	require.NoError(t, d.del.SendRecipient(d.t.Context(), d.s, alice))

	cancelled, err := d.cancel(fixtures.BroadcastSendingID)
	require.NoError(t, err)

	assert.Equal(t, broadcast.StatusCancelled, cancelled.Broadcast.Status)
	assert.Equal(t, 1, cancelled.Unsent)
	first := d.deliver(carol, firstAttempt)
	last := d.deliver(carol, lastAttempt)
	assert.Equal(t, broadcasts.Outcome{}, first, "a queued job of a cancelled Broadcast is done")
	assert.Equal(t, broadcasts.Outcome{}, last, "its last attempt gives nothing up")
	assert.Len(t, d.box.sent, 1, "only the message sent before Cancel went out")
	assert.Zero(t, d.messages(carol))
	assert.Equal(t, broadcast.StatusCancelled, d.status(fixtures.BroadcastSendingID))
	assert.Equal(t, broadcasts.Stats{Recipients: 2, Sent: 1, Cancelled: 1}, d.stats())
}

// A plan job that runs after Cancel is stale: it plans nothing.
func TestAPlanOfACancelledBroadcastIsDone(t *testing.T) {
	d := newDelivery(t, time.Now)
	_, err := d.cancel(fixtures.BroadcastSendingID)
	require.NoError(t, err)

	out, sched := d.runQueued(fixtures.BroadcastSendingID, nil)

	assert.Equal(t, broadcasts.Outcome{}, out)
	assert.Empty(t, sched.sends)
	assert.Zero(t, d.recipientCount(fixtures.BroadcastSendingID))
	assert.Equal(t, broadcast.StatusCancelled, d.status(fixtures.BroadcastSendingID))
}

// A claim racing the move loses: Cancel lands after the send job read the Broadcast as sending and
// right after its claim was written, and the message still does not go out.
func TestAClaimRacingCancelLoses(t *testing.T) {
	d := newDelivery(t, time.Now)
	alice := d.recipient(fixtures.ContactAliceID)
	cancelled := make(chan broadcasts.Cancelled, 1)
	d.env.DB.OutboundMessage.Use(cancelAfterTheClaim(d, cancelled))

	require.NoError(t, d.del.SendRecipient(d.t.Context(), d.s, alice))

	answer := testhelper.Receive(t, cancelled, "Cancel landed after the claim")
	assert.Equal(t, broadcast.StatusCancelled, answer.Broadcast.Status)
	assert.Empty(t, d.box.sent, "the claim lost: the Recipient was left unsent")
	assert.Zero(t, d.messages(alice))
	assert.Equal(t, broadcasts.Stats{Recipients: 1, Cancelled: 1}, d.stats())
}

// A failure that races Cancel loses too: a Recipient whose Contact has no address is failed by a
// row of its own, and a Cancel that lands right after that row is written withdraws it.
func TestAFailureRacingCancelLoses(t *testing.T) {
	d := newDelivery(t, time.Now)
	d.env.Feed(t, func(tx *ent.Client) error {
		return tx.Contact.UpdateOneID(fixtures.ContactAliceID).ClearEmail().Exec(d.t.Context())
	})
	alice := d.recipient(fixtures.ContactAliceID)
	cancelled := make(chan broadcasts.Cancelled, 1)
	d.env.DB.OutboundMessage.Use(cancelAfterTheClaim(d, cancelled))

	require.NoError(t, d.del.SendRecipient(d.t.Context(), d.s, alice))

	answer := testhelper.Receive(t, cancelled, "Cancel landed after the failure")
	assert.Equal(t, broadcast.StatusCancelled, answer.Broadcast.Status)
	assert.Zero(t, d.messages(alice))
	assert.Equal(t, broadcasts.Stats{Recipients: 1, Cancelled: 1}, d.stats())
}

// A Skipped that races Cancel loses too: a Recipient Send-eligibility forbids (Alice opted out of
// Broadcasts) is Skipped before any claim, and a Cancel that lands right after its row is written
// withdraws it.
func TestASkippedRacingCancelLoses(t *testing.T) {
	d := newDelivery(t, time.Now)
	d.env.UnsubscribeFromBroadcasts(d.t.Context(), fixtures.ContactAliceEmail)
	alice := d.recipient(fixtures.ContactAliceID)
	cancelled := make(chan broadcasts.Cancelled, 1)
	d.env.DB.OutboundMessage.Use(cancelAfterTheClaim(d, cancelled))

	require.NoError(t, d.del.SendRecipient(d.t.Context(), d.s, alice))

	answer := testhelper.Receive(t, cancelled, "Cancel landed after the Skipped row")
	assert.Equal(t, broadcast.StatusCancelled, answer.Broadcast.Status)
	assert.Zero(t, d.messages(alice))
	assert.Equal(t, broadcasts.Stats{Recipients: 1, Cancelled: 1}, d.stats())
}

// A plan that Cancel overtakes while it writes the Recipients schedules none of them: they read as
// cancelled.
func TestAPlanRacingCancelSchedulesNothing(t *testing.T) {
	d := newDelivery(t, time.Now)
	cancelled := make(chan broadcasts.Cancelled, 1)
	d.env.DB.BroadcastRecipient.Use(cancelAfterTheClaim(d, cancelled))

	out, sched := d.runQueued(fixtures.BroadcastSendingID, nil)

	testhelper.Receive(t, cancelled, "Cancel landed after the first Recipients were written")
	assert.Equal(t, broadcasts.Outcome{}, out, "the plan job is stale")
	assert.Empty(t, sched.sends)
	planned := d.recipientCount(fixtures.BroadcastSendingID)
	assert.Positive(t, planned)
	assert.Equal(t, broadcasts.Stats{Recipients: planned, Cancelled: planned}, d.stats())
}

// cancelAfterTheClaim cancels the sending Broadcast once, right after the first row the hooked
// client creates (an Outbound message, a Recipient), and hands over what Cancel answered.
func cancelAfterTheClaim(d *delivery, cancelled chan<- broadcasts.Cancelled) ent.Hook {
	done := false
	return func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			v, err := next.Mutate(ctx, m)
			if err != nil || done || !m.Op().Is(ent.OpCreate) {
				return v, err
			}
			done = true
			answer, cerr := d.module().Cancel(ctx, d.s, fixtures.BroadcastSendingID)
			if cerr != nil {
				return v, cerr
			}
			cancelled <- answer
			return v, nil
		})
	}
}

// A message claimed before Cancel goes on: a provider cannot be called back, and the claim is the
// point of no return. Cancel does not count it among the unsent, and its send ends it as any other
// (Bob opted out of Broadcasts, so it ends skipped, not withdrawn).
func TestAMessageClaimedBeforeCancelGoesOn(t *testing.T) {
	d := newDelivery(t, time.Now)
	bob := d.recipient(fixtures.ContactBobID)
	d.claim(bob, time.Hour) // an attempt that died after its claim: its lease has run out

	cancelled, err := d.cancel(fixtures.BroadcastSendingID)
	require.NoError(t, err)
	require.NoError(t, d.del.SendRecipient(d.t.Context(), d.s, bob))

	assert.Zero(t, cancelled.Unsent)
	assert.Equal(t, outboundmessage.StatusSkipped, d.messageOf(bob).Status)
	assert.Equal(t, broadcasts.Stats{Recipients: 1, Skipped: 1}, d.stats())
}

// A claim written after Cancel whose withdrawal never finished (its attempt died between the claim
// and the question) is not taken over once its lease runs out: it is withdrawn, and the Recipient
// reads as cancelled.
func TestAClaimLeftBehindAfterCancelIsWithdrawn(t *testing.T) {
	d := newDelivery(t, time.Now)
	bob := d.recipient(fixtures.ContactBobID)
	_, err := d.cancel(fixtures.BroadcastSendingID)
	require.NoError(t, err)
	d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastSendingID).SetCancelledAt(time.Now().Add(-2 * time.Hour)).ExecX(d.t.Context())
	d.claim(bob, time.Hour) // written an hour after Cancel, its lease run out

	require.NoError(t, d.del.SendRecipient(d.t.Context(), d.s, bob))

	assert.Empty(t, d.box.sent)
	assert.Zero(t, d.messages(bob))
	assert.Equal(t, broadcasts.Stats{Recipients: 1, Cancelled: 1}, d.stats())
}

// A held Broadcast is sending: Cancel ends it like any other, and drops the hold reason, which no
// later job writes back.
func TestAHeldBroadcastIsCancelled(t *testing.T) {
	d := newDelivery(t, time.Now)
	alice := d.recipient(fixtures.ContactAliceID)
	d.env.SuspendWorkspace(d.t.Context(), fixtures.AcmeID, fixedNow)
	held := d.deliver(alice, firstAttempt)
	require.NotZero(t, held.Wait, "the suspension holds the send")

	cancelled, err := d.cancel(fixtures.BroadcastSendingID)
	require.NoError(t, err)
	after := d.deliver(alice, firstAttempt)

	assert.Equal(t, broadcast.StatusCancelled, cancelled.Broadcast.Status)
	assert.Equal(t, 1, cancelled.Unsent)
	assert.Equal(t, broadcasts.Outcome{}, after, "the job of a cancelled Broadcast is done, held or not")
	assert.Nil(t, d.broadcastRow().HoldReason)
}

// Only a sending Broadcast is cancelled: a draft, a scheduled, a sent, a failed and a cancelled one
// are refused, and keep their status.
func TestCancelIsRefusedOutsideSending(t *testing.T) {
	d := newDelivery(t, time.Now)
	_, err := d.cancel(fixtures.BroadcastSendingID)
	require.NoError(t, err)
	refused := map[int64]broadcast.Status{
		fixtures.BroadcastDraftID:     broadcast.StatusDraft,
		fixtures.BroadcastScheduledID: broadcast.StatusScheduled,
		fixtures.BroadcastSentID:      broadcast.StatusSent,
		fixtures.BroadcastFailedID:    broadcast.StatusFailed,
		fixtures.BroadcastSendingID:   broadcast.StatusCancelled,
	}

	for id, status := range refused {
		_, err := d.cancel(id)

		require.ErrorIs(t, err, broadcasts.ErrNotSending, "broadcast %d", id)
		assert.Equal(t, status, d.status(id), "broadcast %d", id)
	}
}

// A cancelled Broadcast is finished: it can be archived, as a sent or failed one can.
func TestACancelledBroadcastIsArchived(t *testing.T) {
	d := newDelivery(t, time.Now)
	_, err := d.cancel(fixtures.BroadcastSendingID)
	require.NoError(t, err)

	archived, err := d.module().Archive(d.t.Context(), d.s, fixtures.BroadcastSendingID)
	require.NoError(t, err)

	assert.NotNil(t, archived.ArchivedAt)
}

// The Recipients left unsent are counted, never written: a cancelled Broadcast has no row for them.
func TestCancelWritesNoRowForTheUnsent(t *testing.T) {
	d := newDelivery(t, time.Now)
	alice := d.recipient(fixtures.ContactAliceID)

	_, err := d.cancel(fixtures.BroadcastSendingID)
	require.NoError(t, err)

	rows, err := d.env.DB.BroadcastRecipient.Query().
		Where(broadcastrecipient.BroadcastID(fixtures.BroadcastSendingID)).IDs(d.t.Context())
	require.NoError(t, err)
	assert.Equal(t, []int64{alice}, rows)
	assert.Zero(t, d.messages(alice))
}
