package outboundmessages_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A provider reply meaning "sending too fast" or "daily quota exceeded" is a
// Deferral with a backoff (ADR 0023), observed at the Outbound send seam with a
// fake provider.

func TestProviderTooFastIsADeferralThenSentExactlyOnce(t *testing.T) {
	env := testhelper.Setup(t)
	m := newModule(env, time.Now)

	env.CustomerMail.SetErr(messaging.ErrBusy)
	res := send(t, m, env, "bc:fast")
	assert.Equal(t, outboundmessages.Deferral, res.Outcome)
	assert.Equal(t, outboundmessages.DeferralBackoff, res.Wait)
	assert.Empty(t, res.Reason)

	n, err := acme(env).OutboundMessage().Query().Where(outboundmessage.IdempotencyKey("bc:fast")).Count(context.Background())
	require.NoError(t, err)
	assert.Zero(t, n, "no claim is left behind: nothing failed, no attempt is spent")

	env.CustomerMail.SetErr(nil)
	retry := send(t, m, env, "bc:fast")
	assert.Equal(t, outboundmessages.Sent, retry.Outcome)
	assert.Len(t, env.CustomerMail.Messages(), 1, "the logical send reached the provider once")

	again := send(t, m, env, "bc:fast")
	assert.Equal(t, outboundmessages.Sent, again.Outcome)
	assert.True(t, again.Replayed)
	assert.Len(t, env.CustomerMail.Messages(), 1, "idempotency still holds after deferral and retry")
}

func TestProviderDailyQuotaIsADeferralWithALongerBackoff(t *testing.T) {
	env := testhelper.Setup(t)
	m := newModule(env, time.Now)

	env.CustomerMail.SetErr(messaging.ErrQuotaExceeded)
	res := send(t, m, env, "bc:quota")
	assert.Equal(t, outboundmessages.Deferral, res.Outcome)
	assert.Equal(t, outboundmessages.QuotaBackoff, res.Wait)
	assert.Greater(t, outboundmessages.QuotaBackoff, outboundmessages.DeferralBackoff)
}

func TestWrappedBusyReplyIsStillADeferral(t *testing.T) {
	env := testhelper.Setup(t)
	m := newModule(env, time.Now)

	env.CustomerMail.SetErr(errors.Join(errors.New("ses: send"), messaging.ErrBusy))
	assert.Equal(t, outboundmessages.Deferral, send(t, m, env, "bc:wrapped").Outcome)
}

// Transactional mail never returns a Deferral (ADR 0023): a provider "too fast" reply is
// a retryable error and the claim is released, so the caller's retry starts clean.
func TestTransactionalBusyReplyIsARetryableErrorNotADeferral(t *testing.T) {
	env := testhelper.Setup(t)
	m := newModule(env, time.Now)
	ctx := context.Background()

	env.CustomerMail.SetErr(messaging.ErrBusy)
	res, err := m.Send(ctx, acme(env), transactional("tx:fast", "a@example.com"))
	require.ErrorIs(t, err, messaging.ErrBusy)
	assert.NotEqual(t, outboundmessages.Deferral, res.Outcome)

	env.CustomerMail.SetErr(nil)
	retry, err := m.Send(ctx, acme(env), transactional("tx:fast", "a@example.com"))
	require.NoError(t, err)
	assert.Equal(t, outboundmessages.Sent, retry.Outcome)
	assert.Len(t, env.CustomerMail.Messages(), 1)
}

func TestGenuineProviderFailureStillFailsTheMessage(t *testing.T) {
	env := testhelper.Setup(t)
	c := startClock()
	m := newModule(env, c.now, outboundmessages.WithLease(time.Nanosecond))
	ctx := context.Background()

	env.CustomerMail.SetErr(errors.New("554 mailbox does not exist"))
	_, err := m.Send(ctx, acme(env), marketing(t, env, "bc:perm", fixtures.ContactAliceID))
	require.Error(t, err, "a real failure is still an error, not a Deferral")

	c.elapse(time.Millisecond)
	require.NoError(t, m.MarkFailed(ctx, acme(env), "bc:perm", err))
	assert.Equal(t, outboundmessage.StatusFailed, byKey(t, env, "bc:perm").Status)
}
