package outboundmessages_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestSendTransactionalRefusesARequestThatCannotBeSent(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := newModule(env, time.Now)
	acme := env.DB.Scoped(fixtures.AcmeID)

	_, err := m.SendTransactional(ctx, acme, outboundmessages.TransactionalRequest{TemplateID: fixtures.TemplateWithVariablesID, Destination: "  "})
	require.ErrorIs(t, err, outboundmessages.ErrTransactionalEmptyDestination)

	_, err = m.SendTransactional(ctx, acme, outboundmessages.TransactionalRequest{TemplateID: fixtures.TemplateGlobexID, Destination: "a@example.com"})
	require.ErrorIs(t, err, outboundmessages.ErrTransactionalTemplateNotFound, "another Workspace's Template is not found")

	_, err = m.SendTransactional(ctx, acme, outboundmessages.TransactionalRequest{
		TemplateID:  fixtures.TemplateWithVariablesID,
		Destination: "a@example.com",
		Variables:   map[string]json.RawMessage{"name": json.RawMessage(`{oops`)},
	})
	require.ErrorIs(t, err, outboundmessages.ErrTransactionalInvalidVariables)
	require.ErrorContains(t, err, `variable "name"`)

	left, err := env.DB.OutboundMessage.Query().Where(outboundmessage.Destination("a@example.com")).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, left, "a refused request leaves no send record")
}

// A Transactional send goes to the normalized address with the Template's current content, and is
// recorded as one Transactional Outbound message.
func TestSendTransactionalSendsTheTemplateToTheNormalizedAddress(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	res, err := newModule(env, time.Now).SendTransactional(ctx, env.DB.Scoped(fixtures.AcmeID), outboundmessages.TransactionalRequest{
		TemplateID: fixtures.TemplateWithVariablesID, Destination: "  Receipt@Example.com ",
	})
	require.NoError(t, err)

	msg, err := env.DB.OutboundMessage.Get(ctx, res.MessageID)
	require.NoError(t, err)
	assert.Equal(t, outboundmessages.TransactionalSent, res.Status)
	assert.Equal(t, "receipt@example.com", res.Destination)
	assert.Equal(t, outboundmessage.KindTransactional, msg.Kind)
	assert.Equal(t, "receipt@example.com", msg.Destination)
}
