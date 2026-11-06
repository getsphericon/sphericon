package outboundmessages_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
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

	builders := map[string]func() outboundmessages.Request{
		"broadcast recipient": func() outboundmessages.Request {
			req, err := outboundmessages.BroadcastRecipient(bc, rec, contact)
			require.NoError(t, err)
			return req
		},
		"automation step": func() outboundmessages.Request {
			return outboundmessages.AutomationStep(automation, run, 0, nil, contact, outboundmessages.EmailContent{Subject: "Hi", Body: "<p>Hi</p>"})
		},
		"transactional with a client key": func() outboundmessages.Request {
			return outboundmessages.Transactional(outboundmessages.TransactionalMessage{
				ClientKey: "order-42", Destination: "a@example.com", Content: outboundmessages.EmailContent{Subject: "Receipt", Body: "<p>Hi</p>"},
			})
		},
	}
	for name, build := range builders {
		t.Run(name, func(t *testing.T) {
			m := newModule(env)
			before := len(env.CustomerMail.Messages())
			first, err := m.Send(ctx, s, build())
			require.NoError(t, err)
			require.Equal(t, outboundmessages.Sent, first.Outcome)
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
	msg := outboundmessages.TransactionalMessage{Destination: "a@example.com",
		Content: outboundmessages.EmailContent{Subject: "Receipt", Body: "<p>Hi</p>"}}

	first, err := newModule(env).Send(ctx, s, outboundmessages.Transactional(msg))
	require.NoError(t, err)
	second, err := newModule(env).Send(ctx, s, outboundmessages.Transactional(msg))
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

	recipient, err := outboundmessages.BroadcastRecipient(bc, rec, contact)
	require.NoError(t, err)
	for want, req := range map[string]outboundmessages.Request{
		fmt.Sprintf("broadcast:%d", rec.ID): recipient,
		"automation:1000:2":                 outboundmessages.AutomationStep(automation, run, 2, nil, contact, outboundmessages.EmailContent{Subject: "Hi", Body: "<p>Hi</p>"}),
		"transactional:k1":                  outboundmessages.Transactional(outboundmessages.TransactionalMessage{ClientKey: "k1", Destination: "a@example.com"}),
	} {
		res, err := newModule(env).Send(ctx, s, req)
		require.NoError(t, err)
		require.NotZero(t, res.MessageID, want)
		assert.Equal(t, want, env.DB.OutboundMessage.GetX(ctx, res.MessageID).IdempotencyKey)
	}
}
