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

// An Idempotency-Key repeated with the same request replays the first send, however its variables'
// JSON is written; repeated with a different request (another destination, variables or tracking)
// it is refused, and nothing is sent for it.
func TestSendTransactionalReplaysAKeyOnlyForTheSameRequest(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := newModule(env, time.Now)
	acme := env.DB.Scoped(fixtures.AcmeID)
	request := func(destination, vars string, track bool) outboundmessages.TransactionalRequest {
		var variables map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(vars), &variables))
		return outboundmessages.TransactionalRequest{
			TemplateID: fixtures.TemplateWithVariablesID, Destination: destination, Variables: variables,
			IdempotencyKey: "order-42", Track: track,
		}
	}

	first, err := m.SendTransactional(ctx, acme, request("a@example.com", `{"name": "Ann", "order": {"id": 42, "total": 9}}`, false))
	require.NoError(t, err)
	again, err := m.SendTransactional(ctx, acme, request(" A@Example.com ", `{"order":{"total":9,"id":42},"name":"Ann"}`, false))
	require.NoError(t, err)
	assert.Equal(t, first.MessageID, again.MessageID, "the same request, written differently, replays the first send")

	for name, req := range map[string]outboundmessages.TransactionalRequest{
		"another destination": request("b@example.com", `{"name": "Ann", "order": {"id": 42, "total": 9}}`, false),
		"other variables":     request("a@example.com", `{"name": "Bob", "order": {"id": 42, "total": 9}}`, false),
		"tracking turned on":  request("a@example.com", `{"name": "Ann", "order": {"id": 42, "total": 9}}`, true),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := m.SendTransactional(ctx, acme, req)
			require.ErrorIs(t, err, outboundmessages.ErrTransactionalKeyReused)
		})
	}
	sent, err := env.DB.OutboundMessage.Query().Where(outboundmessage.IdempotencyKey("transactional:order-42")).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, sent)
}
