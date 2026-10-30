package transactional_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/templates"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/transactional"
)

// outcomeSender is the Outbound module's boundary answering a fixed Outcome, for the answers
// the real module never gives a Transactional request.
type outcomeSender outbound.Outcome

func (o outcomeSender) Send(context.Context, *ent.Scoped, outbound.Request) (outbound.Result, error) {
	return outbound.Result{Outcome: outbound.Outcome(o)}, nil
}

func TestSendRefusesARequestThatCannotBeSent(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := transactional.New(templates.New(testhelper.Renderer()), outcomeSender(outbound.Sent))
	acme := env.DB.Scoped(fixtures.AcmeID)

	_, err := m.Send(ctx, acme, transactional.Request{TemplateID: fixtures.TemplateWithVariablesID, Destination: "  "})
	require.ErrorIs(t, err, transactional.ErrEmptyDestination)

	_, err = m.Send(ctx, acme, transactional.Request{TemplateID: fixtures.TemplateGlobexID, Destination: "a@example.com"})
	require.ErrorIs(t, err, transactional.ErrTemplateNotFound, "another Workspace's Template is not found")

	_, err = m.Send(ctx, acme, transactional.Request{
		TemplateID:  fixtures.TemplateWithVariablesID,
		Destination: "a@example.com",
		Variables:   map[string]json.RawMessage{"name": json.RawMessage(`{oops`)},
	})
	require.ErrorIs(t, err, transactional.ErrInvalidVariables)
	require.ErrorContains(t, err, `variable "name"`)

	left, err := env.DB.OutboundMessage.Query().Where(outboundmessage.Destination("a@example.com")).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, left, "a refused request leaves no send record")
}

func TestSendTreatsADeferralAsAnInvariantViolation(t *testing.T) {
	env := testhelper.Setup(t)
	m := transactional.New(templates.New(testhelper.Renderer()), outcomeSender(outbound.Deferral))

	_, err := m.Send(context.Background(), env.DB.Scoped(fixtures.AcmeID), transactional.Request{
		TemplateID: fixtures.TemplateWithVariablesID, Destination: "a@example.com",
	})

	require.ErrorIs(t, err, transactional.ErrInvariant)
}
