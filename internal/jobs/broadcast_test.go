package jobs_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mokevnin/sphericon/ent"
	"github.com/mokevnin/sphericon/ent/broadcast"
	"github.com/mokevnin/sphericon/ent/broadcastrecipient"
	"github.com/mokevnin/sphericon/ent/contact"
	"github.com/mokevnin/sphericon/ent/suppression"
	"github.com/mokevnin/sphericon/ent/unsubscribe"
	"github.com/mokevnin/sphericon/ent/workspace"
	"github.com/mokevnin/sphericon/internal/eligibility"
	"github.com/mokevnin/sphericon/internal/fixtures"
	"github.com/mokevnin/sphericon/internal/jobs"
	"github.com/mokevnin/sphericon/internal/messaging"
	"github.com/mokevnin/sphericon/internal/outbound"
	"github.com/mokevnin/sphericon/internal/testhelper"
	"github.com/mokevnin/sphericon/internal/tracking"
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
	require.Greater(t, eligibleCount, 0, "the workspace has broadcast-eligible contacts")
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

// The verified-domain send gate (ADR 0010, 0015): a broadcast pinned to a From whose
// domain has no verified sending domain is held at plan time — before any recipient
// row exists — not failed: the broadcast keeps its status, records why, and plans
// again once the domain is verified.
func TestPlanBroadcastHoldsOnUnverifiedFromDomain(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	// news.acme.com exists as an *unverified* sending domain (fixture id 2).
	env.DB.Broadcast.UpdateOneID(fixtures.BroadcastDraftID).SetFromEmail("noreply@news.acme.com").ExecX(ctx)

	_, err := jobs.PlanBroadcast(ctx, env.DB, newMod(env, resolvingTo(&fakeSender{})), fixtures.BroadcastDraftID)
	var held *jobs.HeldError
	require.ErrorAs(t, err, &held)
	assert.Equal(t, outbound.HoldUnverifiedDomain, held.Reason)

	got := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID)
	assert.Equal(t, broadcast.StatusDraft, got.Status, "a hold is not a failure: the status is unchanged")
	require.NotNil(t, got.HoldReason)
	assert.Equal(t, outbound.HoldUnverifiedDomain, *got.HoldReason)

	n, err := env.DB.BroadcastRecipient.Query().
		Where(broadcastrecipient.BroadcastID(fixtures.BroadcastDraftID)).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, n, "no recipient rows created while the broadcast is held")

	// Reversible: once the From is on a verified domain the same plan goes through
	// and the hold is cleared.
	env.DB.Broadcast.UpdateOneID(fixtures.BroadcastDraftID).SetFromEmail("noreply@mail.acme.com").ExecX(ctx)
	ids, err := jobs.PlanBroadcast(ctx, env.DB, newMod(env, resolvingTo(&fakeSender{})), fixtures.BroadcastDraftID)
	require.NoError(t, err)
	assert.NotEmpty(t, ids)
	assert.Nil(t, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).HoldReason)
}

// A Workspace suspension that lands while a broadcast is mid-flight pauses it: the
// remaining recipients stay pending (not failed, not skipped) and send after the
// unsuspend. This is why a hold is a separate outcome scope from Skipped.
func TestSuspensionMidBroadcastPausesRecipientsAndResumes(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	fs := &fakeSender{}
	mod := newMod(env, resolvingTo(fs))

	ids, err := jobs.PlanBroadcast(ctx, env.DB, mod, fixtures.BroadcastDraftID)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(ids), 2)
	require.NoError(t, jobs.SendToRecipient(ctx, env.DB, mod, ids[0]))

	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSuspendedAt(time.Now()).SetSuspendedByKind(workspace.SuspendedByKindSystem).ExecX(ctx)
	err = jobs.SendToRecipient(ctx, env.DB, mod, ids[1])
	var held *jobs.HeldError
	require.ErrorAs(t, err, &held)
	assert.Equal(t, outbound.HoldSuspended, held.Reason)
	assert.Equal(t, broadcastrecipient.StatusPending, env.DB.BroadcastRecipient.GetX(ctx, ids[1]).Status)
	require.NotNil(t, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).HoldReason)
	assert.Len(t, fs.sent, 1)

	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).ClearSuspendedAt().ExecX(ctx)
	require.NoError(t, jobs.SendToRecipient(ctx, env.DB, mod, ids[1]))
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

	ids, err := jobs.PlanBroadcast(ctx, env.DB, mod, fixtures.BroadcastDraftID)
	require.NoError(t, err)
	rec := env.DB.BroadcastRecipient.GetX(ctx, ids[0])
	c := env.DB.Contact.GetX(ctx, rec.ContactID)
	env.DB.Unsubscribe.Create().SetWorkspaceID(fixtures.AcmeID).SetChannel("email").
		SetDestination(eligibility.NormalizeDestination(*c.Email)).
		SetSendingSource(eligibility.SourceBroadcasts).ExecX(ctx)

	require.NoError(t, jobs.SendToRecipient(ctx, env.DB, mod, ids[0]))
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

// A recipient is not re-sent when its send job is retried after a committed
// send: the second SendToRecipient is a no-op (no extra message). Drives the
// draft fixture broadcast through the real plan → send → retry flow.
func TestSendToRecipientIsIdempotent(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	fs := &fakeSender{}
	mod := newMod(env, resolvingTo(fs))

	ids, err := jobs.PlanBroadcast(ctx, env.DB, mod, fixtures.BroadcastDraftID)
	require.NoError(t, err)
	require.NotEmpty(t, ids)

	require.NoError(t, jobs.SendToRecipient(ctx, env.DB, mod, ids[0]))
	require.NoError(t, jobs.SendToRecipient(ctx, env.DB, mod, ids[0]))

	assert.Len(t, fs.sent, 1, "a re-run of an already-sent recipient must not re-send")
	assert.Equal(t, 1, countOutboxEvents(t, env, "email.sent", fixtures.BroadcastDraftID), "exactly one email.sent event")
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
	require.Greater(t, eligible, 0)

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

// An empty audience finalizes immediately to sent with zero counters, so a
// broadcast never hangs in "sending" with no recipients to complete it. The
// fixture broadcast 104 targets segment 103, which matches no contact.
func TestPlanBroadcastEmptyAudienceFinalizes(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	ids, err := jobs.PlanBroadcast(ctx, env.DB, newMod(env, resolvingTo(&fakeSender{})), fixtures.BroadcastEmptyAudienceID)
	require.NoError(t, err)
	assert.Empty(t, ids)

	got := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastEmptyAudienceID)
	assert.Equal(t, broadcast.StatusSent, got.Status)
	assert.Equal(t, 0, got.RecipientsTotal)
	assert.Equal(t, 0, got.SentCount)
	require.NotNil(t, got.SentAt)
}

// A delayed job outlives an unschedule: a broadcast back in draft is not due, a
// scheduled one is.
func TestBroadcastDueIsFalseOnlyForADraft(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	due, err := jobs.BroadcastDue(ctx, env.DB, fixtures.BroadcastDraftID, nil)
	require.NoError(t, err)
	assert.False(t, due, "draft: unscheduled after the job was queued")

	due, err = jobs.BroadcastDue(ctx, env.DB, fixtures.BroadcastScheduledID, nil)
	require.NoError(t, err)
	assert.True(t, due)
}

// A delayed job is bound to the schedule it was enqueued for: after a reschedule
// (or unschedule + schedule) the superseded job must not send the broadcast early.
func TestBroadcastDueIsFalseForASupersededScheduledJob(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	oldAt := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastScheduledID).ScheduledAt
	require.NotNil(t, oldAt)
	newAt := oldAt.Add(48 * time.Hour)
	env.DB.Broadcast.UpdateOneID(fixtures.BroadcastScheduledID).SetScheduledAt(newAt).ExecX(ctx)

	due, err := jobs.BroadcastDue(ctx, env.DB, fixtures.BroadcastScheduledID, oldAt)
	require.NoError(t, err)
	assert.False(t, due, "job for the old schedule is superseded")

	due, err = jobs.BroadcastDue(ctx, env.DB, fixtures.BroadcastScheduledID, &newAt)
	require.NoError(t, err)
	assert.True(t, due, "job for the current schedule runs")
}

// A delayed job is also superseded by an immediate send (which clears scheduled_at),
// while a retry of the job that legitimately started the send (status sending, same
// scheduled_at) still runs.
func TestBroadcastDueForADelayedJobAcrossSendAndRetry(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	at := *env.DB.Broadcast.GetX(ctx, fixtures.BroadcastScheduledID).ScheduledAt

	env.DB.Broadcast.UpdateOneID(fixtures.BroadcastScheduledID).SetStatus(broadcast.StatusSending).ExecX(ctx)
	due, err := jobs.BroadcastDue(ctx, env.DB, fixtures.BroadcastScheduledID, &at)
	require.NoError(t, err)
	assert.True(t, due, "retry of the job that started the send")

	env.DB.Broadcast.UpdateOneID(fixtures.BroadcastScheduledID).ClearScheduledAt().ExecX(ctx)
	due, err = jobs.BroadcastDue(ctx, env.DB, fixtures.BroadcastScheduledID, &at)
	require.NoError(t, err)
	assert.False(t, due, "sent now: the delayed job is superseded")
}

// Finalizing an already-sent broadcast is a no-op: the status=sending guard
// keeps sent_at stable across concurrent/repeated finalizers.
func TestFinalizeBroadcastIsIdempotent(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	require.NoError(t, jobs.SendBroadcast(ctx, env.DB, newMod(env, resolvingTo(&fakeSender{})), fixtures.BroadcastSendingID))
	first := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastSendingID)
	require.Equal(t, broadcast.StatusSent, first.Status)
	require.NotNil(t, first.SentAt)

	require.NoError(t, jobs.FinalizeBroadcast(ctx, env.DB, fixtures.BroadcastSendingID))
	second := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastSendingID)
	assert.True(t, first.SentAt.Equal(*second.SentAt), "sent_at must not move on a repeat finalize")
}

func TestSendBroadcastHoldsWithoutProvider(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	// Fixture broadcast 100; the resolver reports no usable provider. That is a
	// reversible hold on the source, not a failed broadcast.
	err := jobs.SendBroadcast(ctx, env.DB, newMod(env, failingResolver(messaging.ErrNoProvider)), fixtures.BroadcastDraftID)
	var held *jobs.HeldError
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

// A template that cannot render affects every recipient, so it is caught when the
// broadcast is planned (ADR 0015): the broadcast fails as a whole, before any
// recipient row exists, instead of failing each recipient one by one.
func TestPlanBroadcastFailsTheWholeBroadcastOnABrokenTemplate(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	env.DB.Broadcast.UpdateOneID(fixtures.BroadcastDraftID).SetSubject("{% if %}broken").ExecX(ctx)

	_, err := jobs.PlanBroadcast(ctx, env.DB, newMod(env, resolvingTo(&fakeSender{})), fixtures.BroadcastDraftID)
	require.Error(t, err)
	assert.NotErrorIs(t, err, outbound.ErrInProgress)

	assert.Equal(t, broadcast.StatusFailed, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).Status)
	n, err := env.DB.BroadcastRecipient.Query().Where(broadcastrecipient.BroadcastID(fixtures.BroadcastDraftID)).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, n, "no recipient rows for a broadcast that cannot render")
}
