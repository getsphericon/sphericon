package broadcasts_test

import (
	"context"
	"errors"
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

func (m *mailbox) DefaultFrom() (address, name string) { return "hello@codebasics.dev", "CodeBasics" }

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

func (p *provider) EmailSender(context.Context, *ent.Scoped, *ent.Integration) (messaging.EmailSender, error) {
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
}

func newDelivery(t *testing.T) *delivery {
	t.Helper()
	env := testhelper.Setup(t)
	box := &mailbox{}
	res := &provider{sender: box}
	out := outbound.New(env.Bus, env.Holds, res, tracking.New("test-secret", "http://local"))
	return &delivery{
		t: t, env: env, box: box, res: res,
		out: out, del: broadcasts.NewDelivery(out),
		s: env.DB.Scoped(fixtures.AcmeID),
	}
}

// newCoreDelivery is newDelivery on the core holds only, for a test inside a testing/synctest
// bubble: the bubble's clock starts in 2000, before the fixture Staff holds lapsed, so the
// Edition's holds would hold every send.
func newCoreDelivery(t *testing.T) *delivery {
	t.Helper()
	env := testhelper.Setup(t)
	box := &mailbox{}
	res := &provider{sender: box}
	out := outbound.New(env.Bus, env.CoreHolds(), res, tracking.New("test-secret", "http://local"))
	return &delivery{
		t: t, env: env, box: box, res: res,
		out: out, del: broadcasts.NewDelivery(out),
		s: env.DB.Scoped(fixtures.AcmeID),
	}
}

// recipient adds a pending recipient row for a Contact to the sending Broadcast.
func (d *delivery) recipient(contactID int64) int64 {
	return d.env.PlaceRecipient(d.t.Context(), fixtures.BroadcastSendingID, contactID)
}

func (d *delivery) recipientRow(id int64) *ent.BroadcastRecipient {
	return d.env.DB.BroadcastRecipient.GetX(d.t.Context(), id)
}

func (d *delivery) broadcastRow() *ent.Broadcast {
	return d.env.DB.Broadcast.GetX(d.t.Context(), fixtures.BroadcastSendingID)
}

// stats is the sending Broadcast's delivery figures, read from its rows.
func (d *delivery) stats() broadcasts.Stats {
	figures, err := broadcasts.StatsOf(d.t.Context(), d.s, fixtures.BroadcastSendingID)
	require.NoError(d.t, err)
	return figures[fixtures.BroadcastSendingID]
}

// pending reports whether the recipient's send has not ended.
func (d *delivery) pending(recipientID int64) bool {
	return d.env.DB.BroadcastRecipient.Query().
		Where(broadcastrecipient.ID(recipientID), broadcasts.Pending()).ExistX(d.t.Context())
}

// messageOf is the one Outbound message of a recipient's send.
func (d *delivery) messageOf(recipientID int64) *ent.OutboundMessage {
	return d.env.DB.OutboundMessage.Query().
		Where(outboundmessage.BroadcastRecipientID(recipientID)).OnlyX(d.t.Context())
}

// claim moves the claim of a placed recipient's pending Outbound message (only Bob's has one) to
// `age` ago: another attempt's claim, live while younger than the lease.
func (d *delivery) claim(recipientID int64, age time.Duration) {
	d.env.ClaimRecipient(d.t.Context(), recipientID, age)
}

func TestSentRecipientIsRecordedAndSettlesTheBroadcast(t *testing.T) {
	d := newDelivery(t)
	id := d.recipient(fixtures.ContactAliceID)
	d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastSendingID).SetHoldReason(holds.ReasonSuspended).ExecX(d.t.Context())

	require.NoError(t, d.del.SendRecipient(d.t.Context(), d.s, id))

	msg := d.messageOf(id)
	assert.Equal(t, outboundmessage.StatusSent, msg.Status)
	assert.NotNil(t, msg.SentAt)
	assert.Len(t, d.box.sent, 1)
	b := d.broadcastRow()
	assert.Nil(t, b.HoldReason, "a send clears the hold reason")
	assert.Equal(t, broadcast.StatusSent, b.Status, "the last pending recipient settles the Broadcast")
	assert.Equal(t, broadcasts.Stats{Recipients: 1, Sent: 1}, d.stats())
}

func TestSendRecipientIsIdempotent(t *testing.T) {
	d := newDelivery(t)
	id := d.recipient(fixtures.ContactAliceID)

	require.NoError(t, d.del.SendRecipient(d.t.Context(), d.s, id))
	require.NoError(t, d.del.SendRecipient(d.t.Context(), d.s, id))

	assert.Len(t, d.box.sent, 1)
}

func TestRecipientWithoutEmailFailsWithAReason(t *testing.T) {
	d := newDelivery(t)
	d.env.DB.Contact.UpdateOneID(fixtures.ContactAliceID).ClearEmail().ExecX(d.t.Context())
	id := d.recipient(fixtures.ContactAliceID)

	require.NoError(t, d.del.SendRecipient(d.t.Context(), d.s, id))

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
	alice := d.env.DB.Contact.GetX(d.t.Context(), fixtures.ContactAliceID)
	d.env.SuppressDestination(d.t.Context(), *alice.Email)

	require.NoError(t, d.del.SendRecipient(d.t.Context(), d.s, id))

	assert.Equal(t, outboundmessage.StatusSkipped, d.messageOf(id).Status)
	assert.Equal(t, broadcasts.Stats{Recipients: 1, Skipped: 1}, d.stats())
	assert.Empty(t, d.box.sent)
}

func TestRecipientThatCannotRenderFails(t *testing.T) {
	d := newDelivery(t)
	id := d.recipient(fixtures.ContactAliceID)
	d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastSendingID).SetSubject("{% if %}broken").ExecX(d.t.Context())

	require.NoError(t, d.del.SendRecipient(d.t.Context(), d.s, id))

	msg := d.messageOf(id)
	assert.Equal(t, outboundmessage.StatusFailed, msg.Status)
	assert.NotNil(t, msg.Reason)
}

// The message goes out through the Integration the Broadcast names, not the default.
func TestRecipientIsSentThroughTheBroadcastsOwnIntegration(t *testing.T) {
	d := newDelivery(t)
	d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastSendingID).SetIntegrationID(fixtures.IntegrationAcmeMailgunID).ExecX(d.t.Context())
	id := d.recipient(fixtures.ContactAliceID)

	require.NoError(t, d.del.SendRecipient(d.t.Context(), d.s, id))

	msg := d.messageOf(id)
	require.NotNil(t, msg.IntegrationID)
	stamped := *msg.IntegrationID
	assert.EqualValues(t, fixtures.IntegrationAcmeMailgunID, stamped)
}

// A named Integration that cannot send holds the recipient: nothing falls back to the default.
func TestRecipientIsHeldWhenTheBroadcastsIntegrationIsDisabled(t *testing.T) {
	d := newDelivery(t)
	d.env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeMailgunID).SetEnabled(false).ExecX(d.t.Context())
	d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastSendingID).SetIntegrationID(fixtures.IntegrationAcmeMailgunID).ExecX(d.t.Context())
	id := d.recipient(fixtures.ContactAliceID)

	err := d.del.SendRecipient(d.t.Context(), d.s, id)

	var held *broadcasts.HeldError
	require.ErrorAs(t, err, &held)
	assert.Equal(t, "no_integration", held.Reason)
	assert.Empty(t, d.box.sent)
	assert.True(t, d.pending(id))
}

func TestHeldRecipientStaysPendingAndRecordsTheHold(t *testing.T) {
	d := newDelivery(t)
	id := d.recipient(fixtures.ContactAliceID)
	d.env.SuspendWorkspace(d.t.Context(), fixtures.AcmeID, fixedNow)

	err := d.del.SendRecipient(d.t.Context(), d.s, id)

	var held *broadcasts.HeldError
	require.ErrorAs(t, err, &held)
	assert.Equal(t, holds.ReasonSuspended, held.Reason)
	assert.True(t, broadcasts.IsRetryLater(err))
	assert.True(t, d.pending(id))
	b := d.broadcastRow()
	require.NotNil(t, b.HoldReason)
	assert.Equal(t, holds.ReasonSuspended, *b.HoldReason)
	assert.Equal(t, broadcast.StatusSending, b.Status, "a hold keeps the status")
}

func TestDeferredRecipientStaysPendingAndSleeps(t *testing.T) {
	d := newDelivery(t)
	d.env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetMaxPerSecond(1).ExecX(d.t.Context())
	// Transactional traffic spends the Integration's capacity and is itself never slowed.
	for _, key := range []string{"tx:1", "tx:2", "tx:3"} {
		res, err := d.out.Send(d.t.Context(), d.s, outbound.Request{
			Kind: outboundmessage.KindTransactional, Key: key, Destination: "a@example.com",
			Subject: "Receipt", Body: "<p>Hi</p>",
		})
		require.NoError(t, err)
		require.Equal(t, outbound.Sent, res.Outcome)
	}
	second := d.recipient(fixtures.ContactAliceID)

	err := d.del.SendRecipient(d.t.Context(), d.s, second)

	var deferred *broadcasts.DeferredError
	require.ErrorAs(t, err, &deferred)
	assert.Positive(t, deferred.Delay)
	assert.True(t, broadcasts.IsRetryLater(err))
	assert.True(t, d.pending(second))
	assert.NotNil(t, d.recipientRow(second).DeferredUntil)
	assert.Nil(t, d.broadcastRow().HoldReason, "busy is not a hold")
}

// The Send rate limit that defers a recipient is the Broadcast's own Integration's: the
// default is unlimited, the named one has its capacity spent.
func TestDeferralFollowsTheBroadcastsOwnIntegration(t *testing.T) {
	d := newDelivery(t)
	mailgun := int64(fixtures.IntegrationAcmeMailgunID)
	d.env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeMailgunID).SetMaxPerSecond(1).ExecX(d.t.Context())
	d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastSendingID).SetIntegrationID(fixtures.IntegrationAcmeMailgunID).ExecX(d.t.Context())
	for _, key := range []string{"tx:1", "tx:2", "tx:3"} {
		res, err := d.out.Send(d.t.Context(), d.s, outbound.Request{
			Kind: outboundmessage.KindTransactional, Key: key, Destination: "a@example.com",
			Subject: "Receipt", Body: "<p>Hi</p>", IntegrationID: &mailgun,
		})
		require.NoError(t, err)
		require.Equal(t, outbound.Sent, res.Outcome)
	}
	id := d.recipient(fixtures.ContactAliceID)

	err := d.del.SendRecipient(d.t.Context(), d.s, id)

	var deferred *broadcasts.DeferredError
	require.ErrorAs(t, err, &deferred)
	assert.True(t, d.pending(id))
}

func TestUnknownRecipientIsNothingToSend(t *testing.T) {
	d := newDelivery(t)
	require.NoError(t, d.del.SendRecipient(d.t.Context(), d.s, 424242))
}

func TestGiveUpFailsTheRecipientWithTheCauseAndSettlesTheBroadcast(t *testing.T) {
	d := newDelivery(t)
	id := d.recipient(fixtures.ContactAliceID)

	require.NoError(t, d.del.GiveUp(d.t.Context(), d.s, id, errors.New("smtp unavailable")))

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
	id := d.recipient(fixtures.ContactBobID)
	d.claim(id, 0)

	err := d.del.GiveUp(d.t.Context(), d.s, id, errors.New("smtp unavailable"))

	require.ErrorIs(t, err, outbound.ErrInProgress)
	assert.True(t, d.pending(id))
	assert.Equal(t, outboundmessage.StatusPending, d.messageOf(id).Status)
	assert.Equal(t, broadcast.StatusSending, d.broadcastRow().Status)
}

func TestGiveUpFailsAnExpiredClaim(t *testing.T) {
	d := newDelivery(t)
	id := d.recipient(fixtures.ContactBobID)
	d.claim(id, time.Hour)

	require.NoError(t, d.del.GiveUp(d.t.Context(), d.s, id, errors.New("smtp unavailable")))

	assert.False(t, d.pending(id))
	assert.Equal(t, outboundmessage.StatusFailed, d.messageOf(id).Status)
}

// A send that reached the provider but whose recipient write was lost is sent, whatever the
// retries that followed said: the recipient follows its message.
func TestGiveUpAfterTheMailLeftKeepsTheRecipientSent(t *testing.T) {
	d := newDelivery(t)
	id := d.recipient(fixtures.ContactAliceID)
	b := d.broadcastRow()
	rec := d.recipientRow(id)
	c := d.env.DB.Contact.GetX(d.t.Context(), fixtures.ContactAliceID)
	res, err := d.out.Send(d.t.Context(), d.s, outbound.BroadcastRecipient(b, rec, c))
	require.NoError(t, err)
	require.Equal(t, outbound.Sent, res.Outcome)

	require.NoError(t, d.del.GiveUp(d.t.Context(), d.s, id, errors.New("late failure")))

	assert.False(t, d.pending(id))
	assert.Equal(t, outboundmessage.StatusSent, d.messageOf(id).Status)
	assert.Len(t, d.box.sent, 1)
}

// A recipient that is already decided keeps its outcome: a late give-up neither fails it nor
// rewrites its message.
func TestAGiveUpAfterTheSendKeepsItsOutcome(t *testing.T) {
	d := newDelivery(t)
	id := d.recipient(fixtures.ContactAliceID)
	require.NoError(t, d.del.SendRecipient(d.t.Context(), d.s, id))

	require.NoError(t, d.del.GiveUp(d.t.Context(), d.s, id, errors.New("late")))

	assert.Equal(t, outboundmessage.StatusSent, d.messageOf(id).Status)
	assert.Equal(t, broadcasts.Stats{Recipients: 1, Sent: 1}, d.stats())
}

func TestHeldErrorMessage(t *testing.T) {
	assert.Equal(t, "send held: no_integration", (&broadcasts.HeldError{Reason: "no_integration"}).Error())
}
