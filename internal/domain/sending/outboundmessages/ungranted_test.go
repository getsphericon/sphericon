package outboundmessages_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The withdrawal by a default turned to dont_track (ADR 0120) goes in batches: a run that stops at
// its budget says so and where, and the next run continues after the last message it reached.
func TestTheWithdrawalByTheDefaultContinuesAfterTheLastMessageItReached(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	m := testhelper.Module[*outboundmessages.Module](env)
	s := env.DB.Scoped(fixtures.GlobexID)
	now := time.Now()

	first, err := m.WithdrawUngranted(ctx, s, outboundmessages.UngrantedWithdrawal{Before: now, Limit: 1, Batches: 1})
	require.NoError(t, err)
	rest, err := m.WithdrawUngranted(ctx, s, outboundmessages.UngrantedWithdrawal{Before: now, After: first.Last, Limit: 1, Batches: 100})
	require.NoError(t, err)

	assert.True(t, first.More, "one batch of one message leaves Globex's other messages")
	assert.False(t, rest.More)
	msg, err := s.OutboundMessage().Get(ctx, fixtures.OutboundMessageGlobexUsageID)
	require.NoError(t, err)
	assert.False(t, engagementtracking.IsTracked(msg))
}

// The withdrawal reaches only the messages created up to the moment the default was turned.
func TestTheWithdrawalByTheDefaultLeavesTheMessagesCreatedAfterTheTurn(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	m := testhelper.Module[*outboundmessages.Module](env)
	s := env.DB.Scoped(fixtures.GlobexID)
	turned := time.Now().Add(-2 * time.Hour) // before the usage message was created (outbound_messages.yml)

	_, err := m.WithdrawUngranted(ctx, s, outboundmessages.UngrantedWithdrawal{Before: turned, Limit: 100, Batches: 1})
	require.NoError(t, err)

	msg, err := s.OutboundMessage().Get(ctx, fixtures.OutboundMessageGlobexUsageID)
	require.NoError(t, err)
	assert.True(t, engagementtracking.IsTracked(msg))
}

// The withdrawal leaves a message no render decided yet to its render, which reads the default it
// finds then.
func TestTheWithdrawalByTheDefaultLeavesAMessageNotRenderedYet(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	m := testhelper.Module[*outboundmessages.Module](env)
	s := env.DB.Scoped(fixtures.GlobexID)

	_, err := m.WithdrawUngranted(ctx, s, outboundmessages.UngrantedWithdrawal{Before: time.Now(), Limit: 100, Batches: 1})
	require.NoError(t, err)

	pending, err := s.OutboundMessage().Get(ctx, fixtures.OutboundMessageGlobexID)
	require.NoError(t, err)
	assert.Nil(t, pending.TrackingWithdrawnAt)
}

// The withdrawal reaches a message rendered tracked whose send has not ended (its provider failed,
// a retry is due), so the message does not stay tracked for good once the retry goes out.
func TestTheWithdrawalByTheDefaultReachesAMessageRenderedTrackedNotSentYet(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	mod := newModule(env, time.Now)
	req := marketing(t, env, "withdrawn-before-retry", fixtures.ContactAliceID)
	req.Track = true
	s := env.DB.Scoped(fixtures.AcmeID)
	env.CustomerMail.SetErr(errors.New("smtp unavailable"))
	_, err := mod.Send(ctx, s, req)
	require.Error(t, err, "the provider failed: the send is retried")
	env.CustomerMail.SetErr(nil)

	setAcmeDefault(t, env, workspace.EngagementTrackingDefaultDontTrack)
	_, err = mod.WithdrawUngranted(ctx, s, outboundmessages.UngrantedWithdrawal{Before: time.Now(), Limit: 1000, Batches: 10})
	require.NoError(t, err)
	res, err := mod.Send(ctx, s, req)
	require.NoError(t, err)

	msg, err := s.OutboundMessage().Get(ctx, res.MessageID)
	require.NoError(t, err)
	assert.Equal(t, outboundmessages.Sent, res.Outcome)
	assert.False(t, engagementtracking.IsTracked(msg))
}
