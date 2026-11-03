package outbound_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The Outbound message keeps the subject the Contact received, merge tags filled (#899), so a
// feed can name the email an Event is about after the template has changed or gone.
func TestASentEmailKeepsItsRenderedSubject(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)

	res, err := newModule(env).Send(ctx, s, outbound.Transactional(outbound.TransactionalMessage{
		Destination: "ada@example.com",
		Variables:   map[string]any{"nick": "countess"},
		Content:     outbound.EmailContent{Subject: "Hi {{ nick }}", Body: htmlBody},
	}))
	require.NoError(t, err)
	require.Equal(t, outbound.Sent, res.Outcome)

	msg := s.OutboundMessage().Query().Where(outboundmessage.ID(res.MessageID)).OnlyX(ctx)
	require.NotNil(t, msg.Subject)
	assert.Equal(t, "Hi countess", *msg.Subject)
}

// A Broadcast recipient's and an Automation step's sends keep the subject as the Contact received
// it, not the template of the Broadcast or the step.
func TestBroadcastAndAutomationSendsKeepTheirRenderedSubject(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)

	rec := env.DB.BroadcastRecipient.GetX(ctx, fixtures.BroadcastRecipientParkedAliceID)
	bc := env.DB.Broadcast.GetX(ctx, rec.BroadcastID)
	contact := env.DB.Contact.GetX(ctx, rec.ContactID)
	automation := env.DB.Automation.GetX(ctx, fixtures.AutomationWelcomeSeriesID)
	run := env.DB.AutomationRun.GetX(ctx, fixtures.AutomationRunWelcomeCompletedID)

	broadcastReq, err := outbound.BroadcastRecipient(bc, rec, contact)
	require.NoError(t, err)
	stepReq := outbound.AutomationStep(automation, run, 0, nil, contact,
		outbound.EmailContent{Subject: "Hi {{ first_name }}, from a step", Body: htmlBody})

	for name, req := range map[string]outbound.Request{"a recipient": broadcastReq, "a step": stepReq} {
		t.Run(name, func(t *testing.T) {
			before := len(env.CustomerMail.Messages())
			res, err := newModule(env).Send(ctx, s, req)
			require.NoError(t, err)
			require.Equal(t, outbound.Sent, res.Outcome)

			msg := s.OutboundMessage().Query().Where(outboundmessage.ID(res.MessageID)).OnlyX(ctx)
			sentMail := env.CustomerMail.Messages()
			require.Len(t, sentMail, before+1)
			require.NotNil(t, msg.Subject)
			assert.Equal(t, sentMail[before].Subject, *msg.Subject, "what the Contact received")
			assert.NotContains(t, *msg.Subject, "{{", "merge tags are filled")
		})
	}
}
