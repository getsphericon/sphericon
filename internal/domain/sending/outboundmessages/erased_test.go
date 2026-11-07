package outboundmessages_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/contacterasure"
)

// A send queued before its Contact was erased must not leave (ADR 0021): the Request
// still carries the Contact as it was loaded, so the chokepoint re-checks that it
// exists, and records nothing that would keep the erased person's address.
func TestSendToAnErasedContactIsDroppedAtTheChokepoint(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	req := marketing(t, env, "bc:erased", fixtures.ContactAliceID)

	require.NoError(t, contacterasure.New(env.Bus).Erase(ctx, s, contacterasure.Identifier{ContactID: fixtures.ContactAliceID},
		contacterasure.Requester{Kind: contacterasure.RequesterAPIToken}))
	before := len(env.CustomerMail.Messages())

	res, err := newModule(env).Send(ctx, s, req)
	require.NoError(t, err)
	assert.Equal(t, outboundmessages.Skipped, res.Outcome)
	assert.Equal(t, outboundmessages.SkipContactErased, res.Reason)
	assert.Len(t, env.CustomerMail.Messages(), before, "nothing reached the provider")
	n, err := env.DB.OutboundMessage.Query().Where(outboundmessage.IdempotencyKey("bc:erased")).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, n, "no claim or record keeps the address")
}

// A send that names its Contact only by id (the Transactional path) cannot carry a
// stale row, so the chokepoint cannot tell an erased Contact from a foreign one: the
// claim refuses both the same way (the scoped client finds no such Contact in the
// Workspace). Nothing reaches the provider and nothing is recorded.
func TestSendToAnErasedContactIdIsRefusedAtTheClaim(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	require.NoError(t, contacterasure.New(env.Bus).Erase(ctx, s, contacterasure.Identifier{ContactID: fixtures.ContactAliceID},
		contacterasure.Requester{Kind: contacterasure.RequesterAPIToken}))
	before := len(env.CustomerMail.Messages())

	req := transactional("tx:erased-id", "alice@example.com")
	req.ContactID = fixtures.ContactAliceID
	_, err := newModule(env).Send(ctx, s, req)

	require.ErrorIs(t, err, ent.ErrNotInWorkspace)
	assert.Len(t, env.CustomerMail.Messages(), before, "nothing reached the provider")
	n, err := env.DB.OutboundMessage.Query().Where(outboundmessage.IdempotencyKey("tx:erased-id")).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, n)
}
