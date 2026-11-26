package outboundmessages_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/sending/holds"
	"github.com/getsphericon/sphericon/internal/domain/sending/messagetoken"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/deferralcause"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An attempt that takes over a lapsed claim holds it with its own wait (ADR 0178): an Automation
// step's send claimed before it waited, whose claim lapses while the step waits on the Send cap, is
// taken over by the step's next attempt, and the decided message keeps since when and why that
// attempt waited.
func TestAClaimTakenOverKeepsTheWaitOfTheAttemptThatTookIt(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	stepID := automationstep.ID("5d1b7f4e-2a6c-5b9d-c3f5-2a4b6c8d0e15")
	since := time.Now().Add(-2 * time.Hour).Truncate(time.Microsecond)
	retry := automationEmailTo(t, env, fixtures.ContactAliceID, stepID, outboundmessages.Waited{Since: since, Cause: deferralcause.SendCap})
	var taken outboundmessages.Result
	var takenErr error
	var m *outboundmessages.Module
	s := &reentrantSender{EmailSender: env.CustomerMail}
	s.onSend = func() {
		// The first attempt outlives its lease, and the step's next attempt takes its claim over.
		env.DB.OutboundMessage.Update().Where(outboundmessage.IdempotencyKey(retry.Key)).
			SetClaimedAt(time.Now().Add(-time.Hour)).ExecX(ctx)
		taken, takenErr = m.Send(ctx, acme(env), retry)
	}
	m = outboundmessages.New(env.Bus, testhelper.Module[*holds.Holds](env), senders{sender: s}, messagetoken.New("s", "http://local", time.Now), env.OutboundModules(env.Bus, time.Now), outboundmessages.Deps{}, outboundmessages.Config{}, time.Now)
	first := automationEmailTo(t, env, fixtures.ContactAliceID, stepID, outboundmessages.Waited{})

	_, err := m.Send(context.Background(), acme(env), first)

	require.ErrorIs(t, err, outboundmessages.ErrInProgress, "the first attempt lost its claim")
	require.NoError(t, takenErr)
	assert.Equal(t, outboundmessages.Sent, taken.Outcome)
	msg := byKey(t, env, retry.Key)
	require.NotNil(t, msg.DeferredSince, "the claim took the wait of the attempt that holds it")
	assert.True(t, since.Equal(*msg.DeferredSince), "since when: %s", msg.DeferredSince)
	assert.Equal(t, deferralcause.SendCap, lo.FromPtr(msg.DeferralCause))
}

// A Broadcast recipient that waited and is then given up before anything was claimed for it keeps
// its wait on the Failed message, as a decided send does (ADR 0178).
func TestARecipientGivenUpBeforeAClaimKeepsItsWait(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	since := time.Now().Add(-3 * time.Hour).Truncate(time.Microsecond)
	id := env.PlaceRecipient(ctx, fixtures.BroadcastSendingID, fixtures.ContactAliceID)
	env.DB.BroadcastRecipient.UpdateOneID(id).SetDeferredSince(since).SetDeferralCause(deferralcause.ProviderLimit).
		SetDeferredUntil(time.Now()).ExecX(ctx)
	rec := env.DB.BroadcastRecipient.GetX(ctx, id)

	res, err := newModule(env, time.Now).FailRecipient(ctx, acme(env), outboundmessages.Failure{Recipient: rec, Cause: errors.New("content did not render")})

	require.NoError(t, err)
	assert.Equal(t, outboundmessages.Failed, res.Outcome)
	msg := env.DB.OutboundMessage.GetX(ctx, res.MessageID)
	require.NotNil(t, msg.DeferredSince)
	assert.True(t, since.Equal(*msg.DeferredSince), "since when: %s", msg.DeferredSince)
	assert.Equal(t, deferralcause.ProviderLimit, lo.FromPtr(msg.DeferralCause))
}
