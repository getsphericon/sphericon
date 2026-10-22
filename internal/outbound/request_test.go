package outbound_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Each constructor derives the same Request for the same logical send, so building it
// twice deduplicates: the second Send replays the first and the provider sees one mail.
func TestRequestConstructorsDeduplicateTheSameLogicalSend(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)

	// A recipient still to be sent (the fixture recipients already have their message).
	rec := env.DB.BroadcastRecipient.GetX(ctx, fixtures.BroadcastRecipientParkedAliceID)
	bc := env.DB.Broadcast.GetX(ctx, rec.BroadcastID)
	contact := env.DB.Contact.GetX(ctx, rec.ContactID)
	automation := env.DB.Automation.GetX(ctx, fixtures.AutomationWelcomeSeriesID)
	run := env.DB.AutomationRun.GetX(ctx, fixtures.AutomationRunWelcomeCompletedID)

	builders := map[string]func() outbound.Request{
		"broadcast recipient": func() outbound.Request { return outbound.BroadcastRecipient(bc, rec, contact) },
		"automation step": func() outbound.Request {
			return outbound.AutomationStep(automation, run, 0, contact, "Hi", "<p>Hi</p>")
		},
		"transactional with a client key": func() outbound.Request {
			return outbound.Transactional(outbound.TransactionalMessage{
				ClientKey: "order-42", Destination: "a@example.com", Subject: "Receipt", Body: "<p>Hi</p>",
			})
		},
	}
	for name, build := range builders {
		t.Run(name, func(t *testing.T) {
			m := newModule(env)
			before := len(env.CustomerMail.Messages())
			first, err := m.Send(ctx, s, build())
			require.NoError(t, err)
			require.Equal(t, outbound.Sent, first.Outcome)
			second, err := m.Send(ctx, s, build())
			require.NoError(t, err)

			assert.True(t, second.Replayed)
			assert.Equal(t, first.MessageID, second.MessageID)
			assert.Len(t, env.CustomerMail.Messages(), before+1)
		})
	}
}

// A Transactional send without a client key is its own send every time.
func TestTransactionalWithoutClientKeyIsAlwaysANewSend(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	msg := outbound.TransactionalMessage{Destination: "a@example.com", Subject: "Receipt", Body: "<p>Hi</p>"}

	first, err := newModule(env).Send(ctx, s, outbound.Transactional(msg))
	require.NoError(t, err)
	second, err := newModule(env).Send(ctx, s, outbound.Transactional(msg))
	require.NoError(t, err)

	assert.False(t, second.Replayed)
	assert.NotEqual(t, first.MessageID, second.MessageID)
}

// Sends already recorded under the old key formats must still deduplicate after an
// upgrade, so the stored formats are pinned here.
func TestConstructorsKeepTheStoredKeyFormats(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	// A recipient still to be sent (the fixture recipients already have their message).
	rec := env.DB.BroadcastRecipient.GetX(ctx, fixtures.BroadcastRecipientParkedAliceID)
	bc := env.DB.Broadcast.GetX(ctx, rec.BroadcastID)
	contact := env.DB.Contact.GetX(ctx, rec.ContactID)
	automation := env.DB.Automation.GetX(ctx, fixtures.AutomationWelcomeSeriesID)
	run := env.DB.AutomationRun.GetX(ctx, fixtures.AutomationRunWelcomeCompletedID)

	for want, req := range map[string]outbound.Request{
		fmt.Sprintf("broadcast:%d", rec.ID): outbound.BroadcastRecipient(bc, rec, contact),
		"automation:1000:2":                 outbound.AutomationStep(automation, run, 2, contact, "Hi", "<p>Hi</p>"),
		"transactional:k1":                  outbound.Transactional(outbound.TransactionalMessage{ClientKey: "k1", Destination: "a@example.com"}),
	} {
		res, err := newModule(env).Send(ctx, s, req)
		require.NoError(t, err)
		require.NotZero(t, res.MessageID, want)
		assert.Equal(t, want, env.DB.OutboundMessage.GetX(ctx, res.MessageID).IdempotencyKey)
	}
}
