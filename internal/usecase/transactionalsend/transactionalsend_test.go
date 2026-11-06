package transactionalsend_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/content/emailtemplates"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/transactionalsend"
)

// outcomeSender is the Outbound module's boundary answering a fixed Outcome, for the answers
// the real module never gives a Transactional request.
type outcomeSender outboundmessages.Outcome

func (o outcomeSender) Send(context.Context, *ent.Scoped, outboundmessages.Request) (outboundmessages.Result, error) {
	return outboundmessages.Result{Outcome: outboundmessages.Outcome(o)}, nil
}

func TestSendRefusesARequestThatCannotBeSent(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := transactionalsend.New(emailtemplates.New(testhelper.Renderer()), outcomeSender(outboundmessages.Sent))
	acme := env.DB.Scoped(fixtures.AcmeID)

	_, err := m.Send(ctx, acme, transactionalsend.Request{TemplateID: fixtures.TemplateWithVariablesID, Destination: "  "})
	require.ErrorIs(t, err, transactionalsend.ErrEmptyDestination)

	_, err = m.Send(ctx, acme, transactionalsend.Request{TemplateID: fixtures.TemplateGlobexID, Destination: "a@example.com"})
	require.ErrorIs(t, err, transactionalsend.ErrTemplateNotFound, "another Workspace's Template is not found")

	_, err = m.Send(ctx, acme, transactionalsend.Request{
		TemplateID:  fixtures.TemplateWithVariablesID,
		Destination: "a@example.com",
		Variables:   map[string]json.RawMessage{"name": json.RawMessage(`{oops`)},
	})
	require.ErrorIs(t, err, transactionalsend.ErrInvalidVariables)
	require.ErrorContains(t, err, `variable "name"`)

	left, err := env.DB.OutboundMessage.Query().Where(outboundmessage.Destination("a@example.com")).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, left, "a refused request leaves no send record")
}

func TestSendTreatsADeferralAsAnInvariantViolation(t *testing.T) {
	env := testhelper.Setup(t)
	m := transactionalsend.New(emailtemplates.New(testhelper.Renderer()), outcomeSender(outboundmessages.Deferral))

	_, err := m.Send(context.Background(), env.DB.Scoped(fixtures.AcmeID), transactionalsend.Request{
		TemplateID: fixtures.TemplateWithVariablesID, Destination: "a@example.com",
	})

	require.ErrorIs(t, err, transactionalsend.ErrInvariant)
}
