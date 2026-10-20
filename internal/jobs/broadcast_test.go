package jobs_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/lifecycle"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/outbound"
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
func (f *fakeSender) DefaultFrom() (string, string) { return "hello@codebasics.dev", "CodeBasics" }

func (f *fakeSender) Send(_ context.Context, msg messaging.EmailMessage) (messaging.Receipt, error) {
	f.sent = append(f.sent, msg)
	return messaging.Receipt{}, nil
}

// erroringSender fails every send, to exercise the delivery-failure path.
type erroringSender struct{}

func (erroringSender) DefaultFrom() (string, string) { return "hello@codebasics.dev", "CodeBasics" }

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

	// Eligibility is derived (ADR 0001): the audience is the workspace contacts
	// minus the ineligible. Bob is unsubscribed from "broadcasts" in the fixtures,
	// so alice + carol remain.
	eligible, err := env.DB.Contact.Query().
		Where(contact.WorkspaceID(fixtures.AcmeID),
			eligibility.Predicate(eligibility.ChannelEmail, eligibility.SourceBroadcasts)).
		All(ctx)
	require.NoError(t, err)
	eligibleCount := len(eligible)
	require.Positive(t, eligibleCount, "the workspace has broadcast-eligible contacts")
	// Bob is unsubscribed from "broadcasts", so he must not be in the eligible set.
	for _, e := range eligible {
		require.NotEqual(t, "bob@example.com", *e.Email, "unsubscribed contact is excluded from the audience")
	}

	// Send the draft fixture broadcast (100).
	fs := &fakeSender{}
	require.NoError(t, jobs.SendBroadcast(ctx, env.DB, newMod(env, resolvingTo(fs)), fixtures.BroadcastDraftID))

	// One message per eligible contact; no unrendered merge tags leak through
	// (substitution itself is covered by the emailrender tests).
	assert.Len(t, fs.sent, eligibleCount)
	for _, m := range fs.sent {
		assert.NotEqual(t, "bob@example.com", m.To, "unsubscribed destination must not be sent to")
		assert.NotContains(t, m.Subject, "{{")
		assert.NotContains(t, m.HTML, "{{")
		assert.NotEmpty(t, m.Text, "text part derived from HTML")
	}

	// Broadcast is marked sent with accurate counters.
	got := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID)
	assert.Equal(t, broadcast.StatusSent, got.Status)
	assert.Equal(t, eligibleCount, got.RecipientsTotal)
	assert.Equal(t, eligibleCount, got.SentCount)
	assert.Equal(t, 0, got.FailedCount)
	require.NotNil(t, got.SentAt)

	// A recipient row exists per eligible contact, all marked sent.
	recs, err := env.DB.BroadcastRecipient.Query().
		Where(broadcastrecipient.BroadcastID(fixtures.BroadcastDraftID)).
		All(ctx)
	require.NoError(t, err)
	assert.Len(t, recs, eligibleCount)
	for _, r := range recs {
		assert.Equal(t, broadcastrecipient.StatusSent, r.Status)
	}

	// Each send publishes an email.sent event onto the transactional outbox, so the
	// send fact reaches the Event log (Events are the source of truth; persist runs
	// off the bus, not under txdb — projection is covered by the events tests). One
	// per sent recipient, no more (exactly-once).
	assert.Equal(t, eligibleCount, countOutboxEvents(t, env, "email.sent", fixtures.BroadcastDraftID))
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

	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSuspendedAt(time.Now()).SetSuspendedByKind(workspace.SuspendedByKindSystem).ExecX(ctx)
	err = deliverRecipient(ctx, env.DB, mod, ids[1])
	var held *broadcasts.HeldError
	require.ErrorAs(t, err, &held)
	assert.Equal(t, outbound.HoldSuspended, held.Reason)
	assert.Equal(t, broadcastrecipient.StatusPending, env.DB.BroadcastRecipient.GetX(ctx, ids[1]).Status)
	require.NotNil(t, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).HoldReason)
	assert.Len(t, fs.sent, 1)

	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).ClearSuspendedAt().ExecX(ctx)
	require.NoError(t, deliverRecipient(ctx, env.DB, mod, ids[1]))
	assert.Equal(t, broadcastrecipient.StatusSent, env.DB.BroadcastRecipient.GetX(ctx, ids[1]).Status)
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
	env.DB.Unsubscribe.Create().SetWorkspaceID(fixtures.AcmeID).SetChannel("email").
		SetDestination(eligibility.NormalizeDestination(*c.Email)).
		SetSendingSource(eligibility.SourceBroadcasts).ExecX(ctx)

	require.NoError(t, deliverRecipient(ctx, env.DB, mod, ids[0]))
	assert.Equal(t, broadcastrecipient.StatusSkipped, env.DB.BroadcastRecipient.GetX(ctx, ids[0]).Status)
	for _, m := range fs.sent {
		assert.NotEqual(t, *c.Email, m.To, "an unsubscribed destination is never sent to")
	}
}

func TestSendBroadcastSkipsSuppressed(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	// Eligible for broadcasts before suppression: alice + carol (bob is
	// unsubscribed from broadcasts in the fixtures).
	eligibleBefore, err := env.DB.Contact.Query().
		Where(contact.WorkspaceID(fixtures.AcmeID),
			eligibility.Predicate(eligibility.ChannelEmail, eligibility.SourceBroadcasts)).
		Count(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, eligibleBefore, 2)

	// Suppress alice (id 1 in fixtures) — a global hard floor on every surface.
	// The suppression is an incidental edge no fixture expresses, so it's created
	// inline; the broadcast itself is a fixture (scheduled broadcast 101).
	alice := env.DB.Contact.GetX(ctx, fixtures.ContactAliceID)
	_, err = env.DB.Suppression.Create().
		SetWorkspaceID(fixtures.AcmeID).
		SetChannel(suppression.ChannelEmail).
		SetDestination(*alice.Email).
		SetReason(suppression.ReasonManual).
		Save(ctx)
	require.NoError(t, err)

	fs := &fakeSender{}
	require.NoError(t, jobs.SendBroadcast(ctx, env.DB, newMod(env, resolvingTo(fs)), fixtures.BroadcastScheduledID))

	// alice gets no message.
	for _, m := range fs.sent {
		assert.NotEqual(t, *alice.Email, m.To, "suppressed address must not be sent to")
	}
	assert.Len(t, fs.sent, eligibleBefore-1)

	// Counters exclude the suppressed contact, and she has no recipient row.
	got := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastScheduledID)
	assert.Equal(t, eligibleBefore-1, got.RecipientsTotal)
	assert.Equal(t, eligibleBefore-1, got.SentCount)
	n, err := env.DB.BroadcastRecipient.Query().
		Where(broadcastrecipient.BroadcastID(fixtures.BroadcastScheduledID), broadcastrecipient.ContactID(alice.ID)).
		Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, n, "no recipient row for a suppressed contact")
}

// A destination unsubscribed from "everything" is excluded from broadcasts even
// though it carries no per-source ("broadcasts") opt-out.
func TestSendBroadcastSkipsUnsubscribedFromEverything(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	// The everything-opt-out contact + unsubscribe are the edge under test and no
	// fixture expresses them, so they're created inline; the broadcast is a fixture
	// (sending broadcast 102).
	c, err := env.DB.Contact.Create().SetWorkspaceID(fixtures.AcmeID).
		SetEmail("gone@everything.test").SetFirstName("Gone").Save(ctx)
	require.NoError(t, err)
	_, err = env.DB.Unsubscribe.Create().
		SetWorkspaceID(fixtures.AcmeID).
		SetChannel(unsubscribe.ChannelEmail).
		SetDestination(*c.Email).
		SetSendingSource(eligibility.SourceEverything).
		SetContactID(c.ID).
		Save(ctx)
	require.NoError(t, err)

	fs := &fakeSender{}
	require.NoError(t, jobs.SendBroadcast(ctx, env.DB, newMod(env, resolvingTo(fs)), fixtures.BroadcastSendingID))

	// Positive control: the eligible audience (alice + carol; bob is broadcasts-
	// unsubscribed) is still delivered to, and the everything-opt-out is excluded.
	require.NotEmpty(t, fs.sent, "eligible contacts must still receive the broadcast")
	got := make([]string, len(fs.sent))
	for i, m := range fs.sent {
		got[i] = m.To
	}
	assert.NotContains(t, got, *c.Email, "everything-unsubscribed destination must not be sent to")
	assert.Contains(t, got, "alice@example.com")
	assert.Contains(t, got, "carol@example.com")
	assert.NotContains(t, got, "bob@example.com")

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
	assert.Equal(t, len(fs.sent), env.DB.Broadcast.GetX(ctx, fixtures.BroadcastProSegmentID).RecipientsTotal)
}

// A send failure marks the recipient rows failed, finalizes the broadcast with
// failed_count == audience (and sent_count 0), and publishes no email.sent event.
func TestSendBroadcastMarksFailedOnSendError(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	eligible, err := env.DB.Contact.Query().
		Where(contact.WorkspaceID(fixtures.AcmeID),
			eligibility.Predicate(eligibility.ChannelEmail, eligibility.SourceBroadcasts)).
		Count(ctx)
	require.NoError(t, err)
	require.Positive(t, eligible)

	require.NoError(t, jobs.SendBroadcast(ctx, env.DB, newMod(env, resolvingTo(erroringSender{})), fixtures.BroadcastScheduledID))

	got := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastScheduledID)
	assert.Equal(t, broadcast.StatusSent, got.Status, "broadcast finalizes even when every send fails")
	assert.Equal(t, eligible, got.RecipientsTotal)
	assert.Equal(t, 0, got.SentCount)
	assert.Equal(t, eligible, got.FailedCount)
	require.NotNil(t, got.SentAt)

	recs, err := env.DB.BroadcastRecipient.Query().
		Where(broadcastrecipient.BroadcastID(fixtures.BroadcastScheduledID)).
		All(ctx)
	require.NoError(t, err)
	assert.Len(t, recs, eligible)
	for _, r := range recs {
		assert.Equal(t, broadcastrecipient.StatusFailed, r.Status)
		require.NotNil(t, r.Error)
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
	return outbound.New(env.Bus, resolver, tracking.New("test-secret", "http://local"))
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
	assert.Len(t, fs.sent, got.SentCount)
}

// A sent Broadcast stays sent when the synchronous adapter is run on it again.
func TestSendBroadcastOnASentBroadcastIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	err := jobs.SendBroadcast(ctx, env.DB, newMod(env, resolvingTo(&fakeSender{})), fixtures.BroadcastSentID)

	require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
	assert.Equal(t, broadcast.StatusSent, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastSentID).Status)
}
