package outbound_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mokevnin/sphericon/ent"
	"github.com/mokevnin/sphericon/ent/outboundmessage"
	"github.com/mokevnin/sphericon/ent/workspace"
	"github.com/mokevnin/sphericon/internal/eligibility"
	"github.com/mokevnin/sphericon/internal/fixtures"
	"github.com/mokevnin/sphericon/internal/messaging"
	"github.com/mokevnin/sphericon/internal/outbound"
	"github.com/mokevnin/sphericon/internal/testhelper"
	"github.com/mokevnin/sphericon/internal/tracking"
)

// Fixtures: Acme workspace; alice (clean), bob (unsubscribed
// from "broadcasts"), contact 104 ethan.data@codebasics.dev (suppressed). The
// default integration's From is on codebasics.dev (verified); news.acme.com is an
// unverified Sending domain.
const (
	suppressd = "ethan.data@codebasics.dev"
)

const mjml = `<mjml><mj-body><mj-section><mj-column><mj-text>Hi {{ first_name }}</mj-text></mj-column></mj-section></mj-body></mjml>`

type senders struct {
	sender messaging.EmailSender
	err    error
}

func (s senders) EmailSender(context.Context, *ent.Scoped) (messaging.EmailSender, error) {
	return s.sender, s.err
}

func newModule(env *testhelper.TestEnv, opts ...outbound.Option) *outbound.Module {
	return outbound.New(env.Bus, senders{sender: env.CustomerMail},
		tracking.New("test-secret", "http://local"), opts...)
}

func transactional(key, to string) outbound.Request {
	return outbound.Request{
		Kind:        outboundmessage.KindTransactional,
		Key:         key,
		Destination: to,
		Subject:     "Receipt {{ order }}",
		Body:        mjml,
		Variables:   map[string]any{"order": "42"},
	}
}

func marketing(t *testing.T, env *testhelper.TestEnv, key string, contactID int64) outbound.Request {
	t.Helper()
	c := env.DB.Contact.GetX(context.Background(), contactID)
	return outbound.Request{
		Kind:        outboundmessage.KindBroadcast,
		Key:         key,
		Destination: *c.Email,
		Contact:     c,
		Source:      eligibility.SourceBroadcasts,
		Subject:     "News",
		Body:        mjml,
	}
}

// byKey loads the Outbound message recorded under key (the fixtures already hold
// other rows, so tests never assume an empty table).
func byKey(t *testing.T, env *testhelper.TestEnv, key string) *ent.OutboundMessage {
	t.Helper()
	return env.DB.OutboundMessage.Query().
		Where(outboundmessage.WorkspaceID(fixtures.AcmeID), outboundmessage.IdempotencyKey(key)).
		OnlyX(context.Background())
}

func sentEvents(t *testing.T, env *testhelper.TestEnv, messageID int64) int {
	t.Helper()
	return env.OutboxCount(t, "email.sent", map[string]any{"outboundMessageId": messageID})
}

func TestTransactionalSendRecordsMessageAndEvent(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	res, err := newModule(env).Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:1", "Someone@Example.com"))
	require.NoError(t, err)
	assert.Equal(t, outbound.Sent, res.Outcome)

	msgs := env.CustomerMail.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, "someone@example.com", msgs[0].To, "destination is normalized")
	assert.Equal(t, "Receipt 42", msgs[0].Subject)
	assert.Equal(t, "hello@codebasics.dev", msgs[0].From, "effective From is the integration's")
	assert.Empty(t, msgs[0].ListUnsubscribeURL, "transactional carries no unsubscribe header")
	assert.NotContains(t, msgs[0].HTML, "unsubscribe")

	row := env.DB.OutboundMessage.GetX(ctx, res.MessageID)
	assert.Equal(t, outboundmessage.StatusSent, row.Status)
	assert.Equal(t, outboundmessage.KindTransactional, row.Kind)
	require.NotNil(t, row.SendingDomain)
	assert.Equal(t, "codebasics.dev", *row.SendingDomain)
	require.NotNil(t, row.ProviderMessageID)
	assert.NotEmpty(t, *row.ProviderMessageID)
	assert.Nil(t, row.SendingSource)
	assert.Equal(t, 1, sentEvents(t, env, res.MessageID), "one email.sent event, in the same transaction")
}

func TestSameKeyReplaysAndSendsOnce(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := newModule(env)

	first, err := m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:replay", "a@example.com"))
	require.NoError(t, err)
	second, err := m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:replay", "a@example.com"))
	require.NoError(t, err)

	assert.Equal(t, outbound.Sent, second.Outcome)
	assert.True(t, second.Replayed)
	assert.Equal(t, first.MessageID, second.MessageID)
	assert.Len(t, env.CustomerMail.Messages(), 1, "the provider is called once")
	assert.Equal(t, 1, sentEvents(t, env, first.MessageID))
}

func TestSuppressedDestinationIsSkippedEvenForTransactional(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	res, err := newModule(env).Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:sup", suppressd))
	require.NoError(t, err)
	assert.Equal(t, outbound.Skipped, res.Outcome)
	assert.Equal(t, eligibility.ReasonSuppressed, res.Reason)
	assert.Empty(t, env.CustomerMail.Messages())
	row := env.DB.OutboundMessage.GetX(ctx, res.MessageID)
	assert.Equal(t, outboundmessage.StatusSkipped, row.Status, "the skip is recorded")
	assert.Zero(t, sentEvents(t, env, res.MessageID))
}

// The reason Send-eligibility is checked per message: an unsubscribe that exists at
// send time wins, whatever the audience looked like when it was planned.
func TestUnsubscribedContactIsSkippedForMarketingButNotTransactional(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := newModule(env)

	res, err := m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), marketing(t, env, "bc:bob", fixtures.ContactBobID))
	require.NoError(t, err)
	assert.Equal(t, outbound.Skipped, res.Outcome)
	assert.Equal(t, eligibility.ReasonUnsubscribedSource, res.Reason)

	// Bob can still receive his own transactional mail (no Sending source).
	res, err = m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:bob", "bob@example.com"))
	require.NoError(t, err)
	assert.Equal(t, outbound.Sent, res.Outcome)
}

func TestMarketingCarriesFooterAndOneClickHeader(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	res, err := newModule(env).Send(ctx, env.DB.Scoped(fixtures.AcmeID), marketing(t, env, "bc:alice", fixtures.ContactAliceID))
	require.NoError(t, err)
	require.Equal(t, outbound.Sent, res.Outcome)

	msg := env.CustomerMail.Messages()[0]
	assert.Contains(t, msg.HTML, "unsubscribe", "footer link")
	assert.Contains(t, msg.ListUnsubscribeURL, "/e/u/", "RFC 8058 one-click URL")
	row := env.DB.OutboundMessage.GetX(ctx, res.MessageID)
	require.NotNil(t, row.SendingSource)
	assert.Equal(t, eligibility.SourceBroadcasts, *row.SendingSource)
	require.NotNil(t, row.ContactID)
	assert.Equal(t, int64(fixtures.ContactAliceID), *row.ContactID)
}

func TestMarketingWithoutTrackerFailsClosed(t *testing.T) {
	env := testhelper.Setup(t)
	m := outbound.New(env.Bus, senders{sender: env.CustomerMail}, nil)

	_, err := m.Send(context.Background(), env.DB.Scoped(fixtures.AcmeID), marketing(t, env, "bc:notracker", fixtures.ContactAliceID))
	require.ErrorIs(t, err, outbound.ErrNoTracker)
	assert.Empty(t, env.CustomerMail.Messages(), "never sent without an unsubscribe link")
}

func TestRenderErrorIsAPermanentFailureOfThatMessage(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	req := transactional("tx:badliquid", "a@example.com")
	req.Subject = "{% if %}broken"
	res, err := newModule(env).Send(ctx, env.DB.Scoped(fixtures.AcmeID), req)
	require.NoError(t, err)
	assert.Equal(t, outbound.Failed, res.Outcome)
	assert.Empty(t, env.CustomerMail.Messages(), "the raw template is never sent")
	assert.Equal(t, outboundmessage.StatusFailed, env.DB.OutboundMessage.GetX(ctx, res.MessageID).Status)
}

func TestSuspendedWorkspaceIsHeldNotConsumed(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := newModule(env)

	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSuspendedAt(time.Now()).SetSuspendedByKind(workspace.SuspendedByKindSystem).
		SetSuspensionReason("complaint rate").ExecX(ctx)
	res, err := m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), marketing(t, env, "bc:held", fixtures.ContactAliceID))
	require.NoError(t, err)
	assert.Equal(t, outbound.Held, res.Outcome)
	assert.Equal(t, outbound.HoldSuspended, res.Reason)
	assert.Zero(t, res.MessageID, "nothing is recorded for a hold")
	assert.Empty(t, env.CustomerMail.Messages())

	// Reversible: after the unfreeze the same Request goes out.
	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).ClearSuspendedAt().ExecX(ctx)
	res, err = m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), marketing(t, env, "bc:held", fixtures.ContactAliceID))
	require.NoError(t, err)
	assert.Equal(t, outbound.Sent, res.Outcome)
}

func TestUnverifiedFromDomainIsHeld(t *testing.T) {
	env := testhelper.Setup(t)

	req := transactional("tx:unverified", "a@example.com")
	req.FromEmail = "noreply@news.acme.com" // fixture domain id 2: unverified
	res, err := newModule(env).Send(context.Background(), env.DB.Scoped(fixtures.AcmeID), req)
	require.NoError(t, err)
	assert.Equal(t, outbound.Held, res.Outcome)
	assert.Equal(t, outbound.HoldUnverifiedDomain, res.Reason)
	assert.Empty(t, env.CustomerMail.Messages())
}

func TestDomainLosingVerificationAtSignTimeIsHeldAndFreesTheClaim(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := newModule(env)

	env.CustomerMail.SetErr(messaging.ErrUnverifiedSendingDomain)
	res, err := m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:race", "a@example.com"))
	require.NoError(t, err)
	assert.Equal(t, outbound.Held, res.Outcome)

	env.CustomerMail.SetErr(nil)
	res, err = m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:race", "a@example.com"))
	require.NoError(t, err)
	assert.Equal(t, outbound.Sent, res.Outcome, "the same Request can run again")
}

func TestNoIntegrationIsHeld(t *testing.T) {
	env := testhelper.Setup(t)
	m := outbound.New(env.Bus, senders{err: messaging.ErrNoProvider}, tracking.New("s", "http://local"))

	res, err := m.Send(context.Background(), env.DB.Scoped(fixtures.AcmeID), transactional("tx:noint", "a@example.com"))
	require.NoError(t, err)
	assert.Equal(t, outbound.Held, res.Outcome)
	assert.Equal(t, outbound.HoldNoIntegration, res.Reason)
}

func TestProviderErrorIsRetryableAndReleasesTheClaim(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := newModule(env)

	env.CustomerMail.SetErr(errors.New("smtp unavailable"))
	_, err := m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:retry", "a@example.com"))
	require.Error(t, err)
	row := byKey(t, env, "tx:retry")
	assert.Equal(t, outboundmessage.StatusPending, row.Status)

	env.CustomerMail.SetErr(nil)
	res, err := m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:retry", "a@example.com"))
	require.NoError(t, err, "a definite provider failure frees the claim immediately")
	assert.Equal(t, outbound.Sent, res.Outcome)
	assert.Equal(t, row.ID, res.MessageID, "the same row is reused")
	assert.Len(t, env.CustomerMail.Messages(), 1)
}

func TestLiveClaimBlocksAndStaleClaimIsTakenOver(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	// Fixture 106: a pending claim abandoned a day ago (a crashed attempt).
	const key = "transactional:fixture-stale-claim"
	req := transactional(key, "lease.demo@codebasics.dev")

	// The same claim, just taken by another attempt (within the lease), blocks us.
	env.DB.OutboundMessage.Update().Where(outboundmessage.IdempotencyKey(key)).
		SetClaimedAt(time.Now()).ExecX(ctx)
	_, err := newModule(env).Send(ctx, env.DB.Scoped(fixtures.AcmeID), req)
	require.ErrorIs(t, err, outbound.ErrInProgress)
	assert.Empty(t, env.CustomerMail.Messages(), "never two concurrent sends")

	// Abandoned long ago, it is adopted.
	env.DB.OutboundMessage.Update().Where(outboundmessage.IdempotencyKey(key)).
		SetClaimedAt(time.Now().Add(-time.Hour)).ExecX(ctx)
	res, err := newModule(env).Send(ctx, env.DB.Scoped(fixtures.AcmeID), req)
	require.NoError(t, err)
	assert.Equal(t, outbound.Sent, res.Outcome)
	assert.Len(t, env.CustomerMail.Messages(), 1)
}

func TestConfirmedOptInIsReadInsideTheRule(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetRequireConfirmedOptIn(true).ExecX(ctx)

	res, err := newModule(env).Send(ctx, env.DB.Scoped(fixtures.AcmeID), marketing(t, env, "bc:unconfirmed", fixtures.ContactAliceID))
	require.NoError(t, err)
	assert.Equal(t, outbound.Skipped, res.Outcome)
	assert.Equal(t, eligibility.ReasonUnconfirmed, res.Reason)
}

func TestPreflightReportsHoldsWithoutRecording(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := newModule(env)

	before := env.DB.OutboundMessage.Query().CountX(ctx)
	hold, err := m.Preflight(ctx, env.DB.Scoped(fixtures.AcmeID), "")
	require.NoError(t, err)
	assert.Empty(t, hold)
	hold, err = m.Preflight(ctx, env.DB.Scoped(fixtures.AcmeID), "x@news.acme.com")
	require.NoError(t, err)
	assert.Equal(t, outbound.HoldUnverifiedDomain, hold)
	assert.Equal(t, before, env.DB.OutboundMessage.Query().CountX(ctx), "a preflight records nothing")
}

func TestMarkFailedOnlyTouchesPendingClaims(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := newModule(env)

	env.CustomerMail.SetErr(errors.New("down"))
	_, err := m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:giveup", "a@example.com"))
	require.Error(t, err)
	require.NoError(t, m.MarkFailed(ctx, env.DB.Scoped(fixtures.AcmeID), "tx:giveup", errors.New("retries exhausted")))
	row := byKey(t, env, "tx:giveup")
	assert.Equal(t, outboundmessage.StatusFailed, row.Status)
	require.NotNil(t, row.Reason)
	assert.Equal(t, "retries exhausted", *row.Reason)
}

func TestSendTestSkipsEligibilityAndRecordsNothingButHonorsTheFreeze(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := newModule(env)
	before := env.DB.OutboundMessage.Query().CountX(ctx)

	// Even a suppressed address can be the target of an explicit author preview.
	res, err := m.SendTest(ctx, env.DB.Scoped(fixtures.AcmeID), outbound.TestRequest{To: suppressd, Subject: "[Test] Hi", Body: mjml,
		Variables: map[string]any{"first_name": "Alex"}})
	require.NoError(t, err)
	assert.Equal(t, outbound.Sent, res.Outcome)
	msgs := env.CustomerMail.Messages()
	require.Len(t, msgs, 1)
	assert.Empty(t, msgs[0].ListUnsubscribeURL, "a preview carries no unsubscribe header")
	assert.Equal(t, before, env.DB.OutboundMessage.Query().CountX(ctx), "a test send records no Outbound message")
	assert.Zero(t, sentEvents(t, env, 0))

	// But a suspended Workspace sends nothing, test or not.
	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSuspendedAt(time.Now()).ExecX(ctx)
	res, err = m.SendTest(ctx, env.DB.Scoped(fixtures.AcmeID), outbound.TestRequest{To: "a@example.com", Subject: "s", Body: mjml})
	require.NoError(t, err)
	assert.Equal(t, outbound.Held, res.Outcome)
	assert.Equal(t, outbound.HoldSuspended, res.Reason)
	assert.Len(t, env.CustomerMail.Messages(), 1)
}

// reentrantSender lets a test run code from inside the provider call, i.e. while the
// attempt holds its claim — the window in which a slow attempt can outlive its lease.
type reentrantSender struct {
	messaging.EmailSender
	onSend func()
}

func (s *reentrantSender) DefaultFrom() (string, string) { return "hello@codebasics.dev", "CodeBasics" }

func (s *reentrantSender) Send(ctx context.Context, msg messaging.EmailMessage) (messaging.Receipt, error) {
	if s.onSend != nil {
		hook := s.onSend
		s.onSend = nil // only the first (outer) call re-enters
		hook()
	}
	return s.EmailSender.Send(ctx, msg)
}

// A slow attempt that outlives its lease must not record over the attempt that took
// the claim over: every write is fenced on the claim the attempt holds, so there is
// exactly one sent record and one email.sent Event even though two provider calls
// happened (a provider call cannot be un-made; the record can be kept consistent).
func TestSlowAttemptCannotRecordOverATakeover(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	var second outbound.Result
	var secondErr error

	var m *outbound.Module
	s := &reentrantSender{EmailSender: env.CustomerMail}
	s.onSend = func() {
		// The outer attempt is "slow": its lease is expired, and a retry takes over.
		env.DB.OutboundMessage.Update().Where(outboundmessage.IdempotencyKey("tx:fence")).
			SetClaimedAt(time.Now().Add(-time.Hour)).ExecX(ctx)
		second, secondErr = m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:fence", "a@example.com"))
	}
	m = outbound.New(env.Bus, senders{sender: s}, tracking.New("s", "http://local"))

	_, err := m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:fence", "a@example.com"))
	require.ErrorIs(t, err, outbound.ErrInProgress, "the slow attempt lost its claim and must not record")

	require.NoError(t, secondErr)
	assert.Equal(t, outbound.Sent, second.Outcome)
	row := byKey(t, env, "tx:fence")
	assert.Equal(t, outboundmessage.StatusSent, row.Status)
	assert.Equal(t, 1, sentEvents(t, env, row.ID), "one email.sent Event, not two")
}

// The claim row is written through the scoped client, so a Contact of another
// Workspace cannot be attributed to the send.
func TestSendRefusesAForeignContactReference(t *testing.T) {
	env := testhelper.Setup(t)
	req := transactional("tx:foreign-contact", "a@example.com")
	req.ContactID = fixtures.ContactGlobexID

	_, err := newModule(env).Send(context.Background(), env.DB.Scoped(fixtures.AcmeID), req)
	require.ErrorIs(t, err, ent.ErrNotInWorkspace)
	assert.Zero(t, env.DB.OutboundMessage.Query().Where(outboundmessage.IdempotencyKey("tx:foreign-contact")).CountX(context.Background()))
}
