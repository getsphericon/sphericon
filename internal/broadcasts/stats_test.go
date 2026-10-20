package broadcasts_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/fixtures"
)

// The figures of several Broadcasts come from their rows in one read; a Broadcast with no
// recipients has zero figures rather than no entry.
func TestStatsOfReadsEachBroadcastsRows(t *testing.T) {
	d := newDelivery(t)

	got, err := broadcasts.StatsOf(d.ctx, d.s, fixtures.BroadcastSentID, fixtures.BroadcastDraftID, fixtures.BroadcastFailedID)
	require.NoError(t, err)

	// Fixtures: 11 recipients, all sent, 5 opened and 2 clicked.
	assert.Equal(t, broadcasts.Stats{Recipients: 11, Sent: 11, Opened: 5, Clicked: 2}, got[fixtures.BroadcastSentID])
	assert.Equal(t, broadcasts.Stats{Recipients: 5, Failed: 3, Skipped: 2}, got[fixtures.BroadcastFailedID])
	assert.Zero(t, got[fixtures.BroadcastDraftID])
}

func TestStatsOfNoBroadcastsIsEmpty(t *testing.T) {
	d := newDelivery(t)

	got, err := broadcasts.StatsOf(d.ctx, d.s)

	require.NoError(t, err)
	assert.Empty(t, got)
}

// The figures are live: while the Broadcast is sending, what has gone out is already counted
// and the rest is pending, not zero until it finishes.
func TestFiguresAreLiveWhileSending(t *testing.T) {
	d := newDelivery(t)
	first := d.recipient(fixtures.ContactAliceID)
	d.recipient(fixtures.ContactBobID)

	require.NoError(t, d.del.SendRecipient(d.ctx, d.s, first))

	assert.Equal(t, broadcasts.Stats{Recipients: 2, Sent: 1, Pending: 1}, d.stats())
	assert.Equal(t, 1, d.stats().Processed())
	assert.Equal(t, broadcast.StatusSending, d.broadcastRow().Status)
}

// A recipient is pending until its message leaves pending: no message, or one still claimed.
func TestPendingMatchesRecipientsWhoseSendHasNotEnded(t *testing.T) {
	d := newDelivery(t)
	unsent := d.recipient(fixtures.ContactAliceID)
	claimed := d.recipient(fixtures.ContactBobID)
	d.claim(claimed, 0)
	sent := d.recipient(fixtures.ContactCarolID)
	require.NoError(t, d.del.SendRecipient(d.ctx, d.s, sent))

	ids, err := d.s.BroadcastRecipient().Query().
		Where(broadcastrecipient.BroadcastID(fixtures.BroadcastSendingID), broadcasts.Pending()).
		IDs(d.ctx)

	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{unsent, claimed}, ids)
}

func TestBroadcastSettlesOnlyWhenNoSendIsPending(t *testing.T) {
	d := newDelivery(t)
	first := d.recipient(fixtures.ContactAliceID)
	second := d.recipient(fixtures.ContactBobID)
	d.claim(second, 0)

	require.NoError(t, d.del.SendRecipient(d.ctx, d.s, first))
	assert.Equal(t, broadcast.StatusSending, d.broadcastRow().Status, "a claimed send is still pending")

	d.env.DB.OutboundMessage.Update().Where(outboundmessage.BroadcastRecipientID(second)).SetStatus(outboundmessage.StatusSent).ExecX(d.ctx) // the claimed send ends
	require.NoError(t, broadcasts.Finalize(d.ctx, d.s, fixtures.BroadcastSendingID))
	assert.Equal(t, broadcast.StatusSent, d.broadcastRow().Status)
	assert.NotNil(t, d.broadcastRow().SentAt)
}

// Opens, clicks and unsubscribes each count once: the call that sets one reports it, a repeat
// does not, so the event is published once.
func TestEngagementIsRecordedOnce(t *testing.T) {
	d := newDelivery(t)
	id := d.recipient(fixtures.ContactAliceID)

	for name, record := range map[string]func() (bool, error){
		"open":  func() (bool, error) { return broadcasts.RecordOpen(d.ctx, d.s, id) },
		"click": func() (bool, error) { return broadcasts.RecordClick(d.ctx, d.s, id) },
		"unsubscribe": func() (bool, error) {
			return broadcasts.RecordUnsubscribe(d.ctx, d.s, fixtures.BroadcastSendingID, fixtures.ContactAliceID)
		},
	} {
		first, err := record()
		require.NoError(t, err, name)
		again, err := record()
		require.NoError(t, err, name)
		assert.True(t, first, name+" is recorded by the first call")
		assert.False(t, again, name+" is not recorded twice")
	}

	assert.Equal(t, broadcasts.Stats{Recipients: 1, Pending: 1, Opened: 1, Clicked: 1, Unsubscribed: 1}, d.stats())
}

func TestEngagementOfAnUnknownRecipientIsNothing(t *testing.T) {
	d := newDelivery(t)

	recorded, err := broadcasts.RecordOpen(d.ctx, d.s, 424242)
	require.NoError(t, err)
	assert.False(t, recorded)

	recorded, err = broadcasts.RecordUnsubscribe(d.ctx, d.s, fixtures.BroadcastSendingID, fixtures.ContactAliceID)
	require.NoError(t, err)
	assert.False(t, recorded, "a Contact that is not a recipient of the Broadcast matches nothing")
}

func TestRatio(t *testing.T) {
	assert.InDelta(t, 0.25, broadcasts.Ratio(1, 4), 1e-9)
	assert.Zero(t, broadcasts.Ratio(3, 0), "a zero denominator is 0, not Inf")
}
