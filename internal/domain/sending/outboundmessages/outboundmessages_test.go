package outboundmessages_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/sending/holds"
	"github.com/getsphericon/sphericon/internal/domain/sending/messagetoken"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Fixtures: Acme workspace; alice (clean), bob (unsubscribed
// from "broadcasts"), contact 104 ethan.data@codebasics.dev (suppressed). The
// default integration's From is on codebasics.dev (verified); news.acme.com is an
// unverified Sending domain.
const (
	suppressd = "ethan.data@codebasics.dev"
)

const htmlBody = `<p>Hi {{ first_name }}</p>`

type senders struct {
	sender messaging.EmailSender
	err    error
}

func (s senders) EmailSender(context.Context, *ent.Scoped, *ent.Integration) (messaging.EmailSender, error) {
	return s.sender, s.err
}

// clock is the module's clock in a test (outboundmessages.New's now): it stands still until the test
// moves it, so the Provider limit and the claim lease are read at a time the test chose.
type clock struct{ at time.Time }

// startClock starts a clock at the real time, so the fixtures' times keep their meaning.
func startClock() *clock { return &clock{at: time.Now()} }

func (c *clock) now() time.Time { return c.at }

// elapse lets d pass on the clock.
func (c *clock) elapse(d time.Duration) { c.at = c.at.Add(d) }

func newModule(env *testhelper.TestEnv, now func() time.Time, opts ...outboundmessages.Option) *outboundmessages.Module {
	return outboundmessages.New(
		env.Bus,
		env.Holds,
		senders{sender: env.CustomerMail},
		messagetoken.New("test-secret", "http://local", time.Now),
		now,
		opts...,
	)
}

func transactional(key, to string) outboundmessages.Request {
	return outboundmessages.Request{
		Key:         key,
		Destination: to,
		Content:     outboundmessages.EmailContent{Subject: "Receipt {{ order }}", Body: htmlBody},
		Variables:   map[string]any{"order": "42"},
		Ref:         outboundmessages.TransactionalRef{},
	}
}

func marketing(t *testing.T, env *testhelper.TestEnv, key string, contactID int64) outboundmessages.Request {
	t.Helper()
	c := env.DB.Contact.GetX(context.Background(), contactID)
	return outboundmessages.Request{
		Key:         key,
		Destination: *c.Email,
		Contact:     c,
		Source:      eligibility.SourceBroadcasts,
		Content:     outboundmessages.EmailContent{Subject: "News", Body: htmlBody},
		Ref:         broadcastIn(t, c.WorkspaceID),
	}
}

// broadcastIn is the Ref of a Broadcast message of the Workspace: a test's own Broadcast send names
// one of the Workspace's sent Broadcasts from the fixtures.
func broadcastIn(t *testing.T, ws int64) outboundmessages.BroadcastRef {
	t.Helper()
	ids := map[int64]int64{
		fixtures.AcmeID:     fixtures.BroadcastRecentSendsID,
		fixtures.InitechID:  fixtures.BroadcastInitechID,
		fixtures.UmbrellaID: fixtures.BroadcastUmbrellaTestSendsID,
		fixtures.PinewoodID: fixtures.BroadcastPinewoodTestSendsID,
	}
	id, ok := ids[ws]
	require.True(t, ok, "no fixture Broadcast for workspace %d", ws)
	return outboundmessages.BroadcastRef{BroadcastID: id}
}

// withEmail is the email req with its content changed by change.
func withEmail(req outboundmessages.Request, change func(*outboundmessages.EmailContent)) outboundmessages.Request {
	content := req.Content.(outboundmessages.EmailContent)
	change(&content)
	req.Content = content
	return req
}

// sentFrom is the email req sent from the address from instead of its Integration's.
func sentFrom(req outboundmessages.Request, from string) outboundmessages.Request {
	return withEmail(req, func(c *outboundmessages.EmailContent) { c.FromEmail = from })
}

// withSubject is the email req with the subject subject.
func withSubject(req outboundmessages.Request, subject string) outboundmessages.Request {
	return withEmail(req, func(c *outboundmessages.EmailContent) { c.Subject = subject })
}

// withBody is the email req with the HTML body body.
func withBody(req outboundmessages.Request, body string) outboundmessages.Request {
	return withEmail(req, func(c *outboundmessages.EmailContent) { c.Body = body })
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
	return env.QueuedCount(t, "email.sent", map[string]any{"outboundMessageId": messageID})
}

func TestTransactionalSendRecordsMessageAndEvent(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	res, err := newModule(env, time.Now).Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:1", "Someone@Example.com"))
	require.NoError(t, err)
	assert.Equal(t, outboundmessages.Sent, res.Outcome)

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
	m := newModule(env, time.Now)

	first, err := m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:replay", "a@example.com"))
	require.NoError(t, err)
	second, err := m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:replay", "a@example.com"))
	require.NoError(t, err)

	assert.Equal(t, outboundmessages.Sent, second.Outcome)
	assert.True(t, second.Replayed)
	assert.Equal(t, first.MessageID, second.MessageID)
	assert.Len(t, env.CustomerMail.Messages(), 1, "the provider is called once")
	assert.Equal(t, 1, sentEvents(t, env, first.MessageID))
}

func TestSuppressedDestinationIsSkippedEvenForTransactional(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	res, err := newModule(env, time.Now).Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:sup", suppressd))
	require.NoError(t, err)
	assert.Equal(t, outboundmessages.Skipped, res.Outcome)
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
	m := newModule(env, time.Now)

	res, err := m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), marketing(t, env, "bc:bob", fixtures.ContactBobID))
	require.NoError(t, err)
	assert.Equal(t, outboundmessages.Skipped, res.Outcome)
	assert.Equal(t, eligibility.ReasonUnsubscribedSource, res.Reason)

	// Bob can still receive his own transactional mail (no Sending source).
	res, err = m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:bob", "bob@example.com"))
	require.NoError(t, err)
	assert.Equal(t, outboundmessages.Sent, res.Outcome)
}

func TestMarketingCarriesFooterAndOneClickHeader(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	res, err := newModule(env, time.Now).Send(ctx, env.DB.Scoped(fixtures.AcmeID), marketing(t, env, "bc:alice", fixtures.ContactAliceID))
	require.NoError(t, err)
	require.Equal(t, outboundmessages.Sent, res.Outcome)

	msg := env.CustomerMail.Messages()[0]
	assert.Contains(t, msg.HTML, "unsubscribe", "footer link")
	assert.Contains(t, msg.ListUnsubscribeURL, "/e/u/", "RFC 8058 one-click URL")
	row := env.DB.OutboundMessage.GetX(ctx, res.MessageID)
	require.NotNil(t, row.SendingSource)
	assert.Equal(t, eligibility.SourceBroadcasts, *row.SendingSource)
	require.NotNil(t, row.ContactID)
	assert.Equal(t, int64(fixtures.ContactAliceID), *row.ContactID)
	require.NotNil(t, row.AppOrigin, "the origin its links were built on, for `sphericon doctor` (ADR 0036)")
	assert.Equal(t, "http://local", *row.AppOrigin)
}

func TestMarketingWithoutTrackerFailsClosed(t *testing.T) {
	env := testhelper.Setup(t)
	m := outboundmessages.New(env.Bus, env.Holds, senders{sender: env.CustomerMail}, nil, time.Now)

	_, err := m.Send(context.Background(), env.DB.Scoped(fixtures.AcmeID), marketing(t, env, "bc:notracker", fixtures.ContactAliceID))
	require.ErrorIs(t, err, outboundmessages.ErrNoTracker)
	assert.Empty(t, env.CustomerMail.Messages(), "never sent without an unsubscribe link")
}

func TestRenderErrorIsAPermanentFailureOfThatMessage(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	req := transactional("tx:badliquid", "a@example.com")
	req = withSubject(req, "{% if %}broken")
	res, err := newModule(env, time.Now).Send(ctx, env.DB.Scoped(fixtures.AcmeID), req)
	require.NoError(t, err)
	assert.Equal(t, outboundmessages.Failed, res.Outcome)
	assert.Empty(t, env.CustomerMail.Messages(), "the raw template is never sent")
	assert.Equal(t, outboundmessage.StatusFailed, env.DB.OutboundMessage.GetX(ctx, res.MessageID).Status)
}

func TestSuspendedWorkspaceIsHeldNotConsumed(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := newModule(env, time.Now)

	env.SuspendWorkspace(ctx, fixtures.AcmeID, time.Now())
	res, err := m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), marketing(t, env, "bc:held", fixtures.ContactAliceID))
	require.NoError(t, err)
	assert.Equal(t, outboundmessages.Held, res.Outcome)
	assert.Equal(t, holds.ReasonSuspended, res.Reason)
	assert.Zero(t, res.MessageID, "nothing is recorded for a hold")
	assert.Empty(t, env.CustomerMail.Messages())

	// Reversible: after the unfreeze the same Request goes out.
	env.UnsuspendWorkspace(ctx, fixtures.AcmeID)
	res, err = m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), marketing(t, env, "bc:held", fixtures.ContactAliceID))
	require.NoError(t, err)
	assert.Equal(t, outboundmessages.Sent, res.Outcome)
}

// A pending-deletion Workspace is frozen by its own Hold source (ADR 0043), not the
// suspension: nothing is consumed, and restoring lets the same Request go out.
func TestPendingDeletionWorkspaceIsHeldNotConsumed(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := newModule(env, time.Now)

	env.FeedDB.Workspace.UpdateOneID(fixtures.AcmeID).SetDiscardedAt(time.Now()).ExecX(ctx)
	res, err := m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), marketing(t, env, "bc:deleted", fixtures.ContactAliceID))
	require.NoError(t, err)
	assert.Equal(t, outboundmessages.Held, res.Outcome)
	assert.Equal(t, holds.ReasonPendingDeletion, res.Reason)
	assert.Zero(t, res.MessageID)
	assert.Empty(t, env.CustomerMail.Messages())

	// The owner's Hold is the same rule: the send gate's reason and wording.
	standing, err := env.Holds.Standing(ctx, env.DB.Scoped(fixtures.AcmeID))
	require.NoError(t, err)
	owner := standing.Holds()
	require.Len(t, owner, 1)
	assert.Equal(t, res.Reason, owner[0].ReasonCode)
	assert.Equal(t, outboundmessages.Detail(res.Reason), owner[0].Message)

	tx, err := m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:deleted", "a@example.com"))
	require.NoError(t, err)
	assert.Equal(t, outboundmessages.Held, tx.Outcome, "transactional sends are refused too")

	env.FeedDB.Workspace.UpdateOneID(fixtures.AcmeID).ClearDiscardedAt().ExecX(ctx)
	res, err = m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), marketing(t, env, "bc:deleted", fixtures.ContactAliceID))
	require.NoError(t, err)
	assert.Equal(t, outboundmessages.Sent, res.Outcome)
}

func TestUnverifiedFromDomainIsHeld(t *testing.T) {
	env := testhelper.Setup(t)

	req := transactional("tx:unverified", "a@example.com")
	req = sentFrom(req, "noreply@news.acme.com") // fixture domain id 2: unverified
	res, err := newModule(env, time.Now).Send(context.Background(), env.DB.Scoped(fixtures.AcmeID), req)
	require.NoError(t, err)
	assert.Equal(t, outboundmessages.Held, res.Outcome)
	assert.Equal(t, outboundmessages.HoldUnverifiedDomain, res.Reason)
	assert.Empty(t, env.CustomerMail.Messages())
}

func TestDomainLosingVerificationAtSignTimeIsHeldAndFreesTheClaim(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := newModule(env, time.Now)

	env.CustomerMail.SetErr(messaging.ErrUnverifiedSendingDomain)
	res, err := m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:race", "a@example.com"))
	require.NoError(t, err)
	assert.Equal(t, outboundmessages.Held, res.Outcome)

	env.CustomerMail.SetErr(nil)
	res, err = m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:race", "a@example.com"))
	require.NoError(t, err)
	assert.Equal(t, outboundmessages.Sent, res.Outcome, "the same Request can run again")
}

func TestNoIntegrationIsHeld(t *testing.T) {
	env := testhelper.Setup(t)
	m := outboundmessages.New(env.Bus, env.Holds, senders{err: messaging.ErrNoProvider}, messagetoken.New("s", "http://local", time.Now), time.Now)

	res, err := m.Send(context.Background(), env.DB.Scoped(fixtures.AcmeID), transactional("tx:noint", "a@example.com"))
	require.NoError(t, err)
	assert.Equal(t, outboundmessages.Held, res.Outcome)
	assert.Equal(t, outboundmessages.HoldNoIntegration, res.Reason)
}

func TestStoppedProviderAccountIsHeldUntilItSendsAgain(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := newModule(env, time.Now)
	env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID).
		SetProviderEnforcement(providers.EnforcementShutdown).ExecX(ctx)

	res, err := m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:stopped", "a@example.com"))
	require.NoError(t, err)
	assert.Equal(t, outboundmessages.Held, res.Outcome)
	assert.Equal(t, outboundmessages.HoldProviderStopped, res.Reason)
	assert.Empty(t, env.CustomerMail.Messages(), "nothing is handed to a stopped account")

	env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID).ClearProviderEnforcement().ExecX(ctx)
	res, err = m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:stopped", "a@example.com"))
	require.NoError(t, err)
	assert.Equal(t, outboundmessages.Sent, res.Outcome, "the same Request goes out once the account sends again")
}

// Fixtures: Meridian's default is on probation and in its provider's sandbox; the send is from
// Meridian's verified domain.
func TestProviderOnProbationStillSends(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	req := withEmail(transactional("tx:probation", "a@example.com"), func(c *outboundmessages.EmailContent) {
		c.FromEmail = "hello@" + fixtures.SendingDomainMeridianDomain
	})

	res, err := newModule(env, time.Now).Send(ctx, env.DB.Scoped(fixtures.MeridianID), req)
	require.NoError(t, err)
	assert.Equal(t, outboundmessages.Sent, res.Outcome, "probation and the sandbox are warnings, not holds")
}

func TestProviderErrorIsRetryableAndReleasesTheClaim(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := newModule(env, time.Now)

	env.CustomerMail.SetErr(errors.New("smtp unavailable"))
	_, err := m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:retry", "a@example.com"))
	require.Error(t, err)
	row := byKey(t, env, "tx:retry")
	assert.Equal(t, outboundmessage.StatusPending, row.Status)

	env.CustomerMail.SetErr(nil)
	res, err := m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:retry", "a@example.com"))
	require.NoError(t, err, "a definite provider failure frees the claim immediately")
	assert.Equal(t, outboundmessages.Sent, res.Outcome)
	assert.Equal(t, row.ID, res.MessageID, "the same row is reused")
	assert.Len(t, env.CustomerMail.Messages(), 1)
}

func TestLiveClaimBlocksAndStaleClaimIsTakenOver(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	// The fixture stale claim: a pending claim abandoned a day ago (a crashed attempt).
	const key = "transactional:fixture-stale-claim"
	req := transactional(key, "lease.demo@codebasics.dev")

	// The same claim, just taken by another attempt (within the lease), blocks us.
	env.ClaimOutbound(ctx, fixtures.AcmeID, fixtures.OutboundMessageStaleClaimID, time.Now())
	_, err := newModule(env, time.Now).Send(ctx, env.DB.Scoped(fixtures.AcmeID), req)
	require.ErrorIs(t, err, outboundmessages.ErrInProgress)
	assert.Empty(t, env.CustomerMail.Messages(), "never two concurrent sends")

	// Abandoned long ago, it is adopted.
	env.ClaimOutbound(ctx, fixtures.AcmeID, fixtures.OutboundMessageStaleClaimID, time.Now().Add(-time.Hour))
	res, err := newModule(env, time.Now).Send(ctx, env.DB.Scoped(fixtures.AcmeID), req)
	require.NoError(t, err)
	assert.Equal(t, outboundmessages.Sent, res.Outcome)
	assert.Len(t, env.CustomerMail.Messages(), 1)
}

func TestConfirmedOptInIsReadInsideTheRule(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetRequireConfirmedOptIn(true).ExecX(ctx)

	res, err := newModule(env, time.Now).Send(ctx, env.DB.Scoped(fixtures.AcmeID), marketing(t, env, "bc:unconfirmed", fixtures.ContactAliceID))
	require.NoError(t, err)
	assert.Equal(t, outboundmessages.Skipped, res.Outcome)
	assert.Equal(t, eligibility.ReasonUnconfirmed, res.Reason)
}

func TestPreflightReportsHoldsWithoutRecording(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := newModule(env, time.Now)

	before := env.DB.OutboundMessage.Query().CountX(ctx)
	hold, err := m.Preflight(ctx, env.DB.Scoped(fixtures.AcmeID), outboundmessages.EmailContent{}, nil)
	require.NoError(t, err)
	assert.Empty(t, hold)
	hold, err = m.Preflight(ctx, env.DB.Scoped(fixtures.AcmeID), outboundmessages.EmailContent{FromEmail: "x@news.acme.com"}, nil)
	require.NoError(t, err)
	assert.Equal(t, outboundmessages.HoldUnverifiedDomain, hold)
	assert.Equal(t, before, env.DB.OutboundMessage.Query().CountX(ctx), "a preflight records nothing")
}

func TestMarkFailedOnlyTouchesPendingClaims(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := newModule(env, time.Now)

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
	m := newModule(env, time.Now)
	before := env.DB.OutboundMessage.Query().CountX(ctx)

	// Even a suppressed address can be the target of an explicit author preview.
	res, err := m.SendTest(ctx, env.DB.Scoped(fixtures.AcmeID), outboundmessages.TestRequest{To: suppressd,
		Content:   outboundmessages.EmailContent{Subject: "[Test] Hi", Body: htmlBody},
		Variables: map[string]any{"first_name": "Alex"}})
	require.NoError(t, err)
	assert.Equal(t, outboundmessages.Sent, res.Outcome)
	msgs := env.CustomerMail.Messages()
	require.Len(t, msgs, 1)
	assert.Empty(t, msgs[0].ListUnsubscribeURL, "a preview carries no unsubscribe header")
	assert.Equal(t, before, env.DB.OutboundMessage.Query().CountX(ctx), "a test send records no Outbound message")
	assert.Zero(t, sentEvents(t, env, 0))

	// But a suspended Workspace sends nothing, test or not.
	env.SuspendWorkspace(ctx, fixtures.AcmeID, time.Now())
	res, err = m.SendTest(ctx, env.DB.Scoped(fixtures.AcmeID), outboundmessages.TestRequest{To: "a@example.com",
		Content: outboundmessages.EmailContent{Subject: "s", Body: htmlBody}})
	require.NoError(t, err)
	assert.Equal(t, outboundmessages.Held, res.Outcome)
	assert.Equal(t, holds.ReasonSuspended, res.Reason)
	assert.Len(t, env.CustomerMail.Messages(), 1)
}

// reentrantSender lets a test run code from inside the provider call, i.e. while the
// attempt holds its claim — the window in which a slow attempt can outlive its lease.
type reentrantSender struct {
	messaging.EmailSender

	onSend func()
}

func (s *reentrantSender) DefaultFrom() (address, name string) {
	return "hello@codebasics.dev", "CodeBasics"
}

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
	var second outboundmessages.Result
	var secondErr error

	var m *outboundmessages.Module
	s := &reentrantSender{EmailSender: env.CustomerMail}
	s.onSend = func() {
		// The outer attempt is "slow": its lease is expired, and a retry takes over.
		env.DB.OutboundMessage.Update().Where(outboundmessage.IdempotencyKey("tx:fence")).
			SetClaimedAt(time.Now().Add(-time.Hour)).ExecX(ctx)
		second, secondErr = m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:fence", "a@example.com"))
	}
	m = outboundmessages.New(env.Bus, env.Holds, senders{sender: s}, messagetoken.New("s", "http://local", time.Now), time.Now)

	_, err := m.Send(ctx, env.DB.Scoped(fixtures.AcmeID), transactional("tx:fence", "a@example.com"))
	require.ErrorIs(t, err, outboundmessages.ErrInProgress, "the slow attempt lost its claim and must not record")

	require.NoError(t, secondErr)
	assert.Equal(t, outboundmessages.Sent, second.Outcome)
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

	_, err := newModule(env, time.Now).Send(context.Background(), env.DB.Scoped(fixtures.AcmeID), req)
	require.ErrorIs(t, err, ent.ErrNotInWorkspace)
	assert.Zero(t, env.DB.OutboundMessage.Query().Where(outboundmessage.IdempotencyKey("tx:foreign-contact")).CountX(context.Background()))
}
