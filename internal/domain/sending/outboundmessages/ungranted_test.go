package outboundmessages_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	assert.False(t, outboundmessages.IsTracked(msg))
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
	assert.True(t, outboundmessages.IsTracked(msg))
}
