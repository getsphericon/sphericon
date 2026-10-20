package broadcasts_test

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/holds"
	"github.com/getsphericon/sphericon/internal/lifecycle"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/tracking"
)

// mailbox is the Workspace's provider: it records what it sends, or fails every send.
type mailbox struct {
	sent []messaging.EmailMessage
	err  error
}

func (m *mailbox) DefaultFrom() (string, string) { return "hello@codebasics.dev", "CodeBasics" }

func (m *mailbox) Send(_ context.Context, msg messaging.EmailMessage) (messaging.Receipt, error) {
	if m.err != nil {
		return messaging.Receipt{}, m.err
	}
	m.sent = append(m.sent, msg)
	return messaging.Receipt{}, nil
}

// provider resolves the Workspace's email provider: the mailbox, or err when set (no usable
// Integration).
type provider struct {
	sender messaging.EmailSender
	err    error
}

func (p *provider) EmailSender(context.Context, *ent.Scoped) (messaging.EmailSender, error) {
	return p.sender, p.err
}

// delivery is a sending Broadcast (the fixture, with no recipients yet) over a mailbox.
type delivery struct {
	t   *testing.T
	env *testhelper.TestEnv
	box *mailbox
	res *provider
	out *outbound.Module
	del *broadcasts.Delivery
	s   *ent.Scoped
	ctx context.Context
}

func newDelivery(t *testing.T) *delivery {
	t.Helper()
	env := testhelper.Setup(t)
	box := &mailbox{}
	res := &provider{sender: box}
	out := outbound.New(env.Bus, res, tracking.New("test-secret", "http://local"))
	return &delivery{
		t: t, env: env, box: box, res: res, ctx: context.Background(),
		out: out, del: broadcasts.NewDelivery(out),
		s: env.DB.Scoped(fixtures.AcmeID),
	}
}

// recipient adds a pending recipient row for a Contact to the sending Broadcast.
func (d *delivery) recipient(contactID int64) int64 {
	rec, err := d.s.BroadcastRecipient().Create().
		SetBroadcastID(fixtures.BroadcastSendingID).SetContactID(contactID).Save(d.ctx)
	require.NoError(d.t, err)
	return rec.ID
}

func (d *delivery) recipientRow(id int64) *ent.BroadcastRecipient {
	return d.env.DB.BroadcastRecipient.GetX(d.ctx, id)
}

func (d *delivery) broadcastRow() *ent.Broadcast {
	return d.env.DB.Broadcast.GetX(d.ctx, fixtures.BroadcastSendingID)
}

// stats is the sending Broadcast's delivery figures, read from its rows.
func (d *delivery) stats() broadcasts.Stats {
	figures, err := broadcasts.StatsOf(d.ctx, d.s, fixtures.BroadcastSendingID)
	require.NoError(d.t, err)
	return figures[fixtures.BroadcastSendingID]
}

// messageOf is the one Outbound message of a recipient's send.
func (d *delivery) messageOf(recipientID int64) *ent.OutboundMessage {
	return d.env.DB.OutboundMessage.Query().
		Where(outboundmessage.BroadcastRecipientID(recipientID)).OnlyX(d.ctx)
}

// claim leaves a pending Outbound message on the recipient's key, claimed `age` ago: another
// attempt's claim, live while younger than the lease.
func (d *delivery) claim(recipientID int64, age time.Duration) {
	d.env.DB.OutboundMessage.Create().
		SetWorkspaceID(fixtures.AcmeID).
		SetKind(outboundmessage.KindBroadcast).
		SetIdempotencyKey("broadcast:" + strconv.FormatInt(recipientID, 10)).
		SetBroadcastID(fixtures.BroadcastSendingID).
		SetBroadcastRecipientID(recipientID).
		SetClaimedAt(time.Now().Add(-age)).
		ExecX(d.ctx)
}

func TestSentRecipientIsRecordedAndSettlesTheBroadcast(t *testing.T) {
	d := newDelivery(t)
	id := d.recipient(fixtures.ContactAliceID)
	d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastSendingID).SetHoldReason(holds.ReasonSuspended).ExecX(d.ctx)

	require.NoError(t, d.del.SendRecipient(d.ctx, d.s, id))

	rec := d.recipientRow(id)
	assert.Equal(t, broadcastrecipient.StatusSent, rec.Status)
	assert.NotNil(t, rec.SentAt)
	assert.NotNil(t, rec.OutboundMessageID)
	assert.Len(t, d.box.sent, 1)
	b := d.broadcastRow()
	assert.Nil(t, b.HoldReason, "a send clears the hold reason")
	assert.Equal(t, broadcast.StatusSent, b.Status, "the last pending recipient settles the Broadcast")
	assert.Equal(t, broadcasts.Stats{Recipients: 1, Sent: 1}, d.stats())
}

func TestSendRecipientIsIdempotent(t *testing.T) {
	d := newDelivery(t)
	id := d.recipient(fixtures.ContactAliceID)

	require.NoError(t, d.del.SendRecipient(d.ctx, d.s, id))
	require.NoError(t, d.del.SendRecipient(d.ctx, d.s, id))

	assert.Len(t, d.box.sent, 1)
}

func TestRecipientWithoutEmailFailsWithAReason(t *testing.T) {
	d := newDelivery(t)
	d.env.DB.Contact.UpdateOneID(fixtures.ContactAliceID).ClearEmail().ExecX(d.ctx)
	id := d.recipient(fixtures.ContactAliceID)

	require.NoError(t, d.del.SendRecipient(d.ctx, d.s, id))

	rec := d.recipientRow(id)
	assert.Equal(t, broadcastrecipient.StatusFailed, rec.Status)
	require.NotNil(t, rec.Error)
	assert.Equal(t, "contact has no email", *rec.Error)
	assert.Equal(t, broadcasts.Stats{Recipients: 1, Failed: 1}, d.stats())
	assert.Empty(t, d.box.sent)

	msg := d.messageOf(id)
	assert.Equal(t, outboundmessage.StatusFailed, msg.Status, "every recipient ends with exactly one message")
	assert.Equal(t, "contact has no email", *msg.Reason)
	assert.EqualValues(t, fixtures.BroadcastSendingID, *msg.BroadcastID)
}

func TestSuppressedRecipientIsSkippedNotFailed(t *testing.T) {
	d := newDelivery(t)
	id := d.recipient(fixtures.ContactAliceID)
	alice := d.env.DB.Contact.GetX(d.ctx, fixtures.ContactAliceID)
	d.env.DB.Suppression.Create().SetWorkspaceID(fixtures.AcmeID).SetChannel("email").
		SetDestination(*alice.Email).SetReason("bounce").ExecX(d.ctx)

	require.NoError(t, d.del.SendRecipient(d.ctx, d.s, id))

	assert.Equal(t, broadcastrecipient.StatusSkipped, d.recipientRow(id).Status)
	assert.Equal(t, broadcasts.Stats{Recipients: 1, Skipped: 1}, d.stats())
	assert.Empty(t, d.box.sent)
}

func TestRecipientThatCannotRenderFails(t *testing.T) {
	d := newDelivery(t)
	id := d.recipient(fixtures.ContactAliceID)
	d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastSendingID).SetSubject("{% if %}broken").ExecX(d.ctx)

	require.NoError(t, d.del.SendRecipient(d.ctx, d.s, id))

	rec := d.recipientRow(id)
	assert.Equal(t, broadcastrecipient.StatusFailed, rec.Status)
	assert.NotNil(t, rec.Error)
}

func TestHeldRecipientStaysPendingAndRecordsTheHold(t *testing.T) {
	d := newDelivery(t)
	id := d.recipient(fixtures.ContactAliceID)
	d.env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSuspendedAt(fixedNow).ExecX(d.ctx)

	err := d.del.SendRecipient(d.ctx, d.s, id)

	var held *broadcasts.HeldError
	require.ErrorAs(t, err, &held)
	assert.Equal(t, holds.ReasonSuspended, held.Reason)
	assert.True(t, broadcasts.IsRetryLater(err))
	assert.Equal(t, broadcastrecipient.StatusPending, d.recipientRow(id).Status)
	b := d.broadcastRow()
	require.NotNil(t, b.HoldReason)
	assert.Equal(t, holds.ReasonSuspended, *b.HoldReason)
	assert.Equal(t, broadcast.StatusSending, b.Status, "a hold keeps the status")
}

func TestDeferredRecipientStaysPendingAndSleeps(t *testing.T) {
	d := newDelivery(t)
	d.env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetMaxPerSecond(1).ExecX(d.ctx)
	// Transactional traffic spends the Integration's capacity and is itself never slowed.
	for _, key := range []string{"tx:1", "tx:2", "tx:3"} {
		res, err := d.out.Send(d.ctx, d.s, outbound.Request{
			Kind: outboundmessage.KindTransactional, Key: key, Destination: "a@example.com",
			Subject: "Receipt", Body: "<p>Hi</p>",
		})
		require.NoError(t, err)
		require.Equal(t, outbound.Sent, res.Outcome)
	}
	second := d.recipient(fixtures.ContactAliceID)

	err := d.del.SendRecipient(d.ctx, d.s, second)

	var deferred *broadcasts.DeferredError
	require.ErrorAs(t, err, &deferred)
	assert.Positive(t, deferred.Delay)
	assert.True(t, broadcasts.IsRetryLater(err))
	rec := d.recipientRow(second)
	assert.Equal(t, broadcastrecipient.StatusPending, rec.Status)
	assert.NotNil(t, rec.DeferredUntil)
	assert.Nil(t, d.broadcastRow().HoldReason, "busy is not a hold")
}

func TestUnknownRecipientIsNothingToSend(t *testing.T) {
	d := newDelivery(t)
	require.NoError(t, d.del.SendRecipient(d.ctx, d.s, 424242))
}

func TestGiveUpFailsTheRecipientWithTheCauseAndSettlesTheBroadcast(t *testing.T) {
	d := newDelivery(t)
	id := d.recipient(fixtures.ContactAliceID)

	require.NoError(t, d.del.GiveUp(d.ctx, d.s, id, errors.New("smtp unavailable")))

	rec := d.recipientRow(id)
	assert.Equal(t, broadcastrecipient.StatusFailed, rec.Status)
	require.NotNil(t, rec.Error)
	assert.Equal(t, "smtp unavailable", *rec.Error)
	assert.Equal(t, broadcast.StatusSent, d.broadcastRow().Status)
	assert.Equal(t, broadcasts.Stats{Recipients: 1, Failed: 1}, d.stats())

	msg := d.messageOf(id)
	assert.Equal(t, outboundmessage.StatusFailed, msg.Status, "a give-up before any claim still records the one message")
	assert.Equal(t, "smtp unavailable", *msg.Reason)
}

// A live claim is another attempt that may still reach the provider: giving up fails nothing,
// so the recipient is not left failed while its mail goes out, and the adapter can ask again.
func TestGiveUpWaitsOutALiveClaim(t *testing.T) {
	d := newDelivery(t)
	id := d.recipient(fixtures.ContactAliceID)
	d.claim(id, 0)

	err := d.del.GiveUp(d.ctx, d.s, id, errors.New("smtp unavailable"))

	require.ErrorIs(t, err, outbound.ErrInProgress)
	assert.Equal(t, broadcastrecipient.StatusPending, d.recipientRow(id).Status)
	assert.Equal(t, outboundmessage.StatusPending, d.messageOf(id).Status)
	assert.Equal(t, broadcast.StatusSending, d.broadcastRow().Status)
}

func TestGiveUpFailsAnExpiredClaim(t *testing.T) {
	d := newDelivery(t)
	id := d.recipient(fixtures.ContactAliceID)
	d.claim(id, time.Hour)

	require.NoError(t, d.del.GiveUp(d.ctx, d.s, id, errors.New("smtp unavailable")))

	assert.Equal(t, broadcastrecipient.StatusFailed, d.recipientRow(id).Status)
	assert.Equal(t, outboundmessage.StatusFailed, d.messageOf(id).Status)
}

// A send that reached the provider but whose recipient write was lost is sent, whatever the
// retries that followed said: the recipient follows its message.
func TestGiveUpAfterTheMailLeftKeepsTheRecipientSent(t *testing.T) {
	d := newDelivery(t)
	id := d.recipient(fixtures.ContactAliceID)
	b := d.broadcastRow()
	rec := d.recipientRow(id)
	c := d.env.DB.Contact.GetX(d.ctx, fixtures.ContactAliceID)
	res, err := d.out.Send(d.ctx, d.s, outbound.BroadcastRecipient(b, rec, c))
	require.NoError(t, err)
	require.Equal(t, outbound.Sent, res.Outcome)

	require.NoError(t, d.del.GiveUp(d.ctx, d.s, id, errors.New("late failure")))

	assert.Equal(t, broadcastrecipient.StatusSent, d.recipientRow(id).Status)
	assert.Equal(t, outboundmessage.StatusSent, d.messageOf(id).Status)
	assert.Len(t, d.box.sent, 1)
}

func TestOnlyAPendingRecipientCanLeavePending(t *testing.T) {
	d := newDelivery(t)
	id := d.recipient(fixtures.ContactAliceID)
	require.NoError(t, d.del.SendRecipient(d.ctx, d.s, id))

	err := d.del.GiveUp(d.ctx, d.s, id, errors.New("late"))

	require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
	assert.Equal(t, broadcastrecipient.StatusSent, d.recipientRow(id).Status)
}

func TestHeldErrorMessage(t *testing.T) {
	assert.Equal(t, "send held: no_integration", (&broadcasts.HeldError{Reason: "no_integration"}).Error())
}
