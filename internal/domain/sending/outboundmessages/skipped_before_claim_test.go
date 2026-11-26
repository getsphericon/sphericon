package outboundmessages_test

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/skipreason"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A send Skipped before the claim (ADR 0178): the targets and Send-eligibility are decided before
// anything is written, and the decided message is the first row under the Request's key.

// skippedSends are sends Skipped before any claim: a Contact with no Destination on the Channel, and
// one Send-eligibility forbids (Bob opted out of Broadcasts).
var skippedSends = map[string]struct {
	contactID int64
	reason    skipreason.Reason
}{
	"no destination":           {fixtures.ContactJobsNoEmailID, skipreason.NoDestination},
	"send-eligibility forbids": {fixtures.ContactBobID, skipreason.UnsubscribedSource},
}

// toContact is a Broadcast message to the Contact's email address, none when it has none.
func toContact(t *testing.T, env *testhelper.TestEnv, key string, contactID int64) outboundmessages.Request {
	t.Helper()
	c := env.DB.Contact.GetX(t.Context(), contactID)
	return outboundmessages.Request{
		Key:         key,
		Destination: lo.FromPtr(c.Email),
		Contact:     c,
		Content:     outboundmessages.EmailContent{Subject: "News", Body: htmlBody},
		Ref:         broadcastIn(t, c.WorkspaceID),
	}
}

// answer is what a send answered.
type answer struct {
	res outboundmessages.Result
	err error
}

// sendAgainAround sends req once more through m when the first Outbound message is written: right
// after its insert when afterIt, else right before it. It hands over what that second send answered.
func sendAgainAround(env *testhelper.TestEnv, m *outboundmessages.Module, s *ent.Scoped, req outboundmessages.Request, afterIt bool) <-chan answer {
	second := make(chan answer, 1)
	done := false
	sendAgain := func(ctx context.Context) {
		done = true
		res, err := m.Send(ctx, s, req)
		second <- answer{res, err}
	}
	env.DB.OutboundMessage.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mu ent.Mutation) (ent.Value, error) {
			if done || !mu.Op().Is(ent.OpCreate) {
				return next.Mutate(ctx, mu)
			}
			if !afterIt {
				sendAgain(ctx)
			}
			v, err := next.Mutate(ctx, mu)
			if afterIt && err == nil {
				sendAgain(ctx)
			}
			return v, err
		})
	})
	return second
}

// A second send under the key that comes right after the first one's write finds a decided message,
// never a claim in progress: both report the one Skipped, and a replay returns it again.
func TestASendSkippedBeforeTheClaimWritesItsDecidedMessageFirst(t *testing.T) {
	for name, tc := range skippedSends {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			s := env.DB.Scoped(fixtures.AcmeID)
			m := newModule(env, time.Now)
			req := toContact(t, env, "skipped-first:"+name, tc.contactID)
			second := sendAgainAround(env, m, s, req, true)

			first, err := m.Send(t.Context(), s, req)
			require.NoError(t, err)
			again := testhelper.Receive(t, second, "the second send ran after the first write")
			replay, rerr := m.Send(t.Context(), s, req)
			require.NoError(t, rerr)

			require.NoError(t, again.err, "the second send found a decided message, not a claim")
			assert.Equal(t, outboundmessages.Skipped, first.Outcome)
			assert.Equal(t, tc.reason, first.SkippedReason)
			assert.Equal(t, []any{outboundmessages.Skipped, tc.reason, first.MessageID}, []any{again.res.Outcome, again.res.SkippedReason, again.res.MessageID})
			assert.Equal(t, []any{outboundmessages.Skipped, tc.reason, first.MessageID}, []any{replay.Outcome, replay.SkippedReason, replay.MessageID})
			row := byKey(t, env, req.Key)
			assert.Equal(t, first.MessageID, row.ID)
			assert.Equal(t, outboundmessage.StatusSkipped, row.Status)
			assert.Equal(t, tc.reason, lo.FromPtr(row.SkippedReason))
			assert.Empty(t, env.CustomerMail.Messages())
		})
	}
}

// Two sends racing to one Skipped decision: the one whose insert loses the key is judged by the row
// that holds it, so one row is left and both report its Outcome.
func TestTwoSendsRacingToASkippedDecisionLeaveOneRow(t *testing.T) {
	for name, tc := range skippedSends {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			s := env.DB.Scoped(fixtures.AcmeID)
			m := newModule(env, time.Now)
			req := toContact(t, env, "skipped-race:"+name, tc.contactID)
			second := sendAgainAround(env, m, s, req, false)

			first, err := m.Send(t.Context(), s, req)
			require.NoError(t, err)
			won := testhelper.Receive(t, second, "the second send ran before the first write")

			require.NoError(t, won.err)
			assert.Equal(t, []any{outboundmessages.Skipped, tc.reason}, []any{won.res.Outcome, won.res.SkippedReason})
			assert.Equal(t, []any{outboundmessages.Skipped, tc.reason, won.res.MessageID}, []any{first.Outcome, first.SkippedReason, first.MessageID})
			n := env.DB.OutboundMessage.Query().Where(outboundmessage.WorkspaceID(fixtures.AcmeID), outboundmessage.IdempotencyKey(req.Key)).CountX(t.Context())
			assert.Equal(t, 1, n)
		})
	}
}

// A pending claim already under the key is judged before anything is written: a live one is a send
// in progress and stays as it is, and a lapsed one is adopted and Skipped.
func TestASkippedDecisionAdoptsAPendingClaimFirst(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)
	const key = "transactional:fixture-stale-claim"
	req := transactional(key, suppressd)
	claimedAt := time.Now().Truncate(time.Microsecond)
	env.ClaimOutbound(ctx, fixtures.AcmeID, fixtures.OutboundMessageStaleClaimID, claimedAt)

	_, err := newModule(env, time.Now).Send(ctx, s, req)
	live := byKey(t, env, key)
	env.ClaimOutbound(ctx, fixtures.AcmeID, fixtures.OutboundMessageStaleClaimID, time.Now().Add(-time.Hour))
	res, rerr := newModule(env, time.Now).Send(ctx, s, req)
	require.NoError(t, rerr)

	require.ErrorIs(t, err, outboundmessages.ErrInProgress)
	assert.Equal(t, outboundmessage.StatusPending, live.Status, "a live claim is left alone")
	assert.True(t, claimedAt.Equal(live.ClaimedAt))
	assert.Equal(t, outboundmessages.Skipped, res.Outcome)
	assert.Equal(t, skipreason.Suppressed, res.SkippedReason)
	assert.Equal(t, int64(fixtures.OutboundMessageStaleClaimID), res.MessageID, "the lapsed claim is the message")
}

// A Skipped decided for a source that has stopped (a cancelled Broadcast, ADR 0150) is withdrawn:
// no row is left under the key.
func TestASkippedDecisionOfAStoppedSourceWritesNoRow(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	req := toContact(t, env, "skipped-stopped", fixtures.ContactBobID)
	stoppedAt := time.Now().Add(-time.Minute)
	req.Stopped = func(context.Context, *ent.Scoped) (*time.Time, error) { return &stoppedAt, nil }

	res, err := newModule(env, time.Now).Send(t.Context(), s, req)
	require.NoError(t, err)

	assert.Equal(t, outboundmessages.Withdrawn, res.Outcome)
	n := env.DB.OutboundMessage.Query().Where(outboundmessage.WorkspaceID(fixtures.AcmeID), outboundmessage.IdempotencyKey(req.Key)).CountX(t.Context())
	assert.Zero(t, n)
}
