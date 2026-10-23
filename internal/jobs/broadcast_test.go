package jobs_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/holds"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/lifecycle"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/suppression"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/tracking"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSender struct {
	sent []messaging.EmailMessage
}

// DefaultFrom is on the fixtures' verified Sending domain, so sends without an
// explicit From pass the verified-domain gate.
func (f *fakeSender) DefaultFrom() (address, name string) {
	return "hello@codebasics.dev", "CodeBasics"
}

func (f *fakeSender) Send(_ context.Context, msg messaging.EmailMessage) (messaging.Receipt, error) {
	f.sent = append(f.sent, msg)
	return messaging.Receipt{}, nil
}

// erroringSender fails every send, to exercise the delivery-failure path.
type erroringSender struct{}

func (erroringSender) DefaultFrom() (address, name string) {
	return "hello@codebasics.dev", "CodeBasics"
}

func (erroringSender) Send(context.Context, messaging.EmailMessage) (messaging.Receipt, error) {
	return messaging.Receipt{}, errors.New("smtp unavailable")
}

// resolvingTo is the Workspace's email provider lookup answering with sender.
func resolvingTo(sender messaging.EmailSender) *SendersMock {
	return &SendersMock{EmailSenderFunc: func(context.Context, *ent.Scoped) (messaging.EmailSender, error) {
		return sender, nil
	}}
}

// failingResolver is the provider lookup failing with err (no usable Integration).
func failingResolver(err error) *SendersMock {
	return &SendersMock{EmailSenderFunc: func(context.Context, *ent.Scoped) (messaging.EmailSender, error) {
		return nil, err
	}}
}

func TestSendBroadcastDeliversToEligibleContacts(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	// Eligibility is derived (ADR 0001): the audience is the workspace contacts minus the
	// ineligible. Bob is unsubscribed from "broadcasts" in the fixtures, the email-less
	// contact has no destination.
	fs := &fakeSender{}
	require.NoError(t, jobs.SendBroadcast(ctx, env.DB, newMod(env, resolvingTo(fs)), fixtures.BroadcastDraftID))

	// One message per delivered contact; no unrendered merge tags leak through
	// (substitution itself is covered by the emailrender tests).
	got := make([]string, len(fs.sent))
	for i, m := range fs.sent {
		got[i] = m.To
		assert.NotContains(t, m.Subject, "{{")
		assert.NotContains(t, m.HTML, "{{")
		assert.NotEmpty(t, m.Text, "text part derived from HTML")
	}
	assert.Contains(t, got, fixtures.ContactAliceEmail)
	assert.Contains(t, got, fixtures.ContactCarolEmail)
	assert.NotContains(t, got, fixtures.ContactBobEmail, "unsubscribed destination must not be sent to")
	sent := len(fs.sent)

	// Broadcast is marked sent with accurate counters.
	done := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID)
	assert.Equal(t, broadcast.StatusSent, done.Status)
	assert.Equal(t, broadcasts.Stats{Recipients: sent, Sent: sent}, figuresOf(t, env, fixtures.BroadcastDraftID))
	require.NotNil(t, done.SentAt)

	// A recipient row exists per delivered contact and none for the email-less one.
	recs, err := env.DB.BroadcastRecipient.Query().
		Where(broadcastrecipient.BroadcastID(fixtures.BroadcastDraftID)).
		All(ctx)
	require.NoError(t, err)
	assert.Len(t, recs, sent)
	assert.False(t, env.DB.BroadcastRecipient.Query().
		Where(broadcastrecipient.BroadcastID(fixtures.BroadcastDraftID), broadcastrecipient.ContactID(fixtures.ContactJobsNoEmailID)).
		ExistX(ctx), "a contact without an email is not a recipient")

	// Each send publishes an email.sent event onto the transactional outbox, so the
	// send fact reaches the Event log (Events are the source of truth; persist runs
	// off the bus, not under txdb — projection is covered by the events tests). One
	// per sent recipient, no more (exactly-once).
	assert.Equal(t, sent, countOutboxEvents(t, env, "email.sent", fixtures.BroadcastDraftID))
}

// countOutboxEvents returns how many events of the given name for the given
// broadcast sit in the transactional outbox.
func countOutboxEvents(t *testing.T, env *testhelper.TestEnv, name string, broadcastID int64) int {
	t.Helper()
	return env.OutboxCount(t, name, map[string]any{"broadcastId": broadcastID})
}

// A Workspace suspension that lands while a broadcast is mid-flight pauses it: the
// remaining recipients stay pending (not failed, not skipped) and send after the
// unsuspend. This is why a hold is a separate outcome scope from Skipped.
func TestSuspensionMidBroadcastPausesRecipientsAndResumes(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	fs := &fakeSender{}
	mod := newMod(env, resolvingTo(fs))

	ids, err := planAudience(ctx, env.DB, mod, fixtures.BroadcastDraftID)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(ids), 2)
	require.NoError(t, deliverRecipient(ctx, env.DB, mod, ids[0]))

	env.SuspendWorkspace(ctx, fixtures.AcmeID, time.Now())
	err = deliverRecipient(ctx, env.DB, mod, ids[1])
	var held *broadcasts.HeldError
	require.ErrorAs(t, err, &held)
	assert.Equal(t, holds.ReasonSuspended, held.Reason)
	assert.True(t, env.DB.BroadcastRecipient.Query().Where(broadcastrecipient.ID(ids[1]), broadcasts.Pending()).ExistX(ctx))
	require.NotNil(t, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).HoldReason)
	assert.Len(t, fs.sent, 1)

	env.UnsuspendWorkspace(ctx, fixtures.AcmeID)
	require.NoError(t, deliverRecipient(ctx, env.DB, mod, ids[1]))
	assert.False(t, env.DB.BroadcastRecipient.Query().Where(broadcastrecipient.ID(ids[1]), broadcasts.Pending()).ExistX(ctx))
	assert.Nil(t, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).HoldReason, "the hold clears when a send goes through")
}

// An unsubscribe that lands after planning but before the send still wins: the
// recipient is skipped at send time (Send-eligibility is checked per message), and
// the broadcast reports it as skipped, not sent and not failed.
func TestUnsubscribeBetweenPlanAndSendSkipsTheRecipient(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	fs := &fakeSender{}
	mod := newMod(env, resolvingTo(fs))

	ids, err := planAudience(ctx, env.DB, mod, fixtures.BroadcastDraftID)
	require.NoError(t, err)
	rec := env.DB.BroadcastRecipient.GetX(ctx, ids[0])
	c := env.DB.Contact.GetX(ctx, rec.ContactID)
	env.UnsubscribeFromBroadcasts(ctx, suppression.NormalizeDestination(*c.Email))

	require.NoError(t, deliverRecipient(ctx, env.DB, mod, ids[0]))
	assert.Equal(t, outboundmessage.StatusSkipped, env.DB.OutboundMessage.Query().Where(outboundmessage.BroadcastRecipientID(ids[0])).OnlyX(ctx).Status)
	for _, m := range fs.sent {
		assert.NotEqual(t, *c.Email, m.To, "an unsubscribed destination is never sent to")
	}
}

func TestSendBroadcastSkipsSuppressed(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	bounced := env.DB.Contact.GetX(ctx, fixtures.ContactBouncedID)
	complained := env.DB.Contact.GetX(ctx, fixtures.ContactComplainedID)

	fs := &fakeSender{}
	require.NoError(t, jobs.SendBroadcast(ctx, env.DB, newMod(env, resolvingTo(fs)), fixtures.BroadcastScheduledID))

	got := make([]string, len(fs.sent))
	for i, m := range fs.sent {
		got[i] = m.To
	}
	assert.Contains(t, got, fixtures.ContactAliceEmail, "an unsuppressed contact still receives the broadcast")
	assert.NotContains(t, got, *bounced.Email, "an address suppressed for a bounce must not be sent to")
	assert.NotContains(t, got, *complained.Email, "an address suppressed for a complaint must not be sent to")

	// The figures count what was delivered, and a suppressed contact has no recipient row.
	assert.Equal(t, broadcasts.Stats{Recipients: len(fs.sent), Sent: len(fs.sent)}, figuresOf(t, env, fixtures.BroadcastScheduledID))
	assert.False(t, env.DB.BroadcastRecipient.Query().
		Where(broadcastrecipient.BroadcastID(fixtures.BroadcastScheduledID), broadcastrecipient.ContactID(bounced.ID)).
		ExistX(ctx), "no recipient row for a suppressed contact")
}

// A destination unsubscribed from "everything" is excluded from broadcasts even
// though it carries no per-source ("broadcasts") opt-out.
func TestSendBroadcastSkipsUnsubscribedFromEverything(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	gone := env.DB.Contact.GetX(ctx, fixtures.ContactUnsubscribedEverythingID)

	fs := &fakeSender{}
	require.NoError(t, jobs.SendBroadcast(ctx, env.DB, newMod(env, resolvingTo(fs)), fixtures.BroadcastSendingID))

	// Positive control: the eligible audience (alice + carol; bob is broadcasts-
	// unsubscribed) is still delivered to, and the everything-opt-out is excluded.
	got := make([]string, len(fs.sent))
	for i, m := range fs.sent {
		got[i] = m.To
	}
	assert.NotContains(t, got, *gone.Email, "everything-unsubscribed destination must not be sent to")
	assert.Contains(t, got, fixtures.ContactAliceEmail)
	assert.Contains(t, got, fixtures.ContactCarolEmail)
	assert.NotContains(t, got, fixtures.ContactBobEmail)

	// Every broadcast carries an RFC 8058 one-click unsubscribe URL (ADR 0012),
	// scoped to the "broadcasts" source (the /e/u/ token endpoint).
	for _, m := range fs.sent {
		assert.Contains(t, m.ListUnsubscribeURL, "/e/u/", "broadcast sets a one-click unsubscribe URL")
	}
}

func TestSendBroadcastToRuleSegment(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	// Fixture broadcast 105 targets fixture rule segment 100 ("plan = pro"), so the
	// audience is exactly the pro-plan fixture contacts (e.g. liam id 100), not the
	// free-plan ones (e.g. noah id 102).
	liam := env.DB.Contact.GetX(ctx, fixtures.ContactLiamID) // plan=pro
	noah := env.DB.Contact.GetX(ctx, fixtures.ContactNoahID) // plan=free

	fs := &fakeSender{}
	require.NoError(t, jobs.SendBroadcast(ctx, env.DB, newMod(env, resolvingTo(fs)), fixtures.BroadcastProSegmentID))

	got := make([]string, len(fs.sent))
	for i, m := range fs.sent {
		got[i] = m.To
	}
	require.NotEmpty(t, got, "pro-plan contacts are in the audience")
	assert.Contains(t, got, *liam.Email, "a pro-plan contact receives the broadcast")
	assert.NotContains(t, got, *noah.Email, "a free-plan contact is excluded by the segment")
	// recipients_total matches what was actually sent (all segment matches eligible).
	assert.Equal(t, len(fs.sent), figuresOf(t, env, fixtures.BroadcastProSegmentID).Recipients)
}

// A send failure marks the recipient rows failed, finalizes the broadcast with
// failed_count == audience (and sent_count 0), and publishes no email.sent event.
func TestSendBroadcastMarksFailedOnSendError(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	require.NoError(t, jobs.SendBroadcast(ctx, env.DB, newMod(env, resolvingTo(erroringSender{})), fixtures.BroadcastScheduledID))

	got := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastScheduledID)
	assert.Equal(t, broadcast.StatusSent, got.Status, "broadcast finalizes even when every send fails")
	require.NotNil(t, got.SentAt)

	recs, err := env.DB.BroadcastRecipient.Query().
		Where(broadcastrecipient.BroadcastID(fixtures.BroadcastScheduledID)).
		All(ctx)
	require.NoError(t, err)
	audience := len(recs)
	require.Positive(t, audience, "the broadcast has an audience")
	assert.Equal(t, broadcasts.Stats{Recipients: audience, Failed: audience}, figuresOf(t, env, fixtures.BroadcastScheduledID))
	msgs, err := env.DB.OutboundMessage.Query().
		Where(outboundmessage.BroadcastID(fixtures.BroadcastScheduledID)).
		All(ctx)
	require.NoError(t, err)
	assert.Len(t, msgs, audience, "every recipient ends with one message, each failed with its cause")
	for _, m := range msgs {
		assert.Equal(t, outboundmessage.StatusFailed, m.Status)
		require.NotNil(t, m.Reason)
	}

	assert.Zero(t, countOutboxEvents(t, env, "email.sent", fixtures.BroadcastScheduledID), "no email.sent event when the send fails")
}

func TestSendBroadcastHoldsWithoutProvider(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	// Fixture broadcast 100; the resolver reports no usable provider. That is a
	// reversible hold on the source, not a failed broadcast.
	err := jobs.SendBroadcast(ctx, env.DB, newMod(env, failingResolver(messaging.ErrNoProvider)), fixtures.BroadcastDraftID)
	var held *broadcasts.HeldError
	require.ErrorAs(t, err, &held)
	assert.Equal(t, outbound.HoldNoIntegration, held.Reason)

	got := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID)
	assert.NotEqual(t, broadcast.StatusFailed, got.Status)
	require.NotNil(t, got.HoldReason)
}

// newMod builds the Outbound send module over a test resolver, with a real tracker
// so marketing sends carry their unsubscribe footer and header.
func newMod(env *testhelper.TestEnv, resolver outbound.Senders) *outbound.Module {
	return outbound.New(env.Bus, env.Holds, resolver, tracking.New("test-secret", "http://local"))
}

// The synchronous adapter runs the production sequence: the module plans, paces and settles
// the Broadcast, so the pacing it records is what the river adapter would schedule by.
func TestSendBroadcastRunsTheProductionSequence(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetMaxPerSecond(10000).ExecX(ctx)
	fs := &fakeSender{}

	require.NoError(t, jobs.SendBroadcast(ctx, env.DB, newMod(env, resolvingTo(fs)), fixtures.BroadcastDraftID))

	got := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID)
	assert.Equal(t, broadcast.StatusSent, got.Status)
	assert.NotNil(t, got.LastScheduledAt, "paced like production")
	assert.Len(t, fs.sent, figuresOf(t, env, fixtures.BroadcastDraftID).Sent)
}

// A sent Broadcast stays sent when the synchronous adapter is run on it again.
func TestSendBroadcastOnASentBroadcastIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	err := jobs.SendBroadcast(ctx, env.DB, newMod(env, resolvingTo(&fakeSender{})), fixtures.BroadcastSentID)

	require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
	assert.Equal(t, broadcast.StatusSent, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastSentID).Status)
}

// figuresOf is a Broadcast's delivery figures, derived from its rows.
func figuresOf(t *testing.T, env *testhelper.TestEnv, broadcastID int64) broadcasts.Stats {
	t.Helper()
	got, err := broadcasts.StatsOf(context.Background(), env.DB.Scoped(fixtures.AcmeID), broadcastID)
	require.NoError(t, err)
	return got[broadcastID]
}

// stillPending reports whether the recipient's send has not ended (see broadcasts.Pending).
func stillPending(ctx context.Context, client *ent.Client, recipientID int64) bool {
	return client.BroadcastRecipient.Query().
		Where(broadcastrecipient.ID(recipientID), broadcasts.Pending()).ExistX(ctx)
}
