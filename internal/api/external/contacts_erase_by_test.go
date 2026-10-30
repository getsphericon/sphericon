package external_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/visitor"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestExternalContactsEraseByEmailAppliesTheRulesOfById(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := initechClient(t, env, "contacts:erase")

	res, err := c.ContactsEraseBy(ctx, externalapi.ContactsEraseByParams{
		Email: externalapi.NewOptEmailAddress(fixtures.ContactErasableEmail),
	})
	require.NoError(t, err)
	require.IsTypef(t, &externalapi.ContactsEraseByNoContent{}, res, "got %T", res)

	gone, err := env.DB.Contact.Query().Where(contact.ID(fixtures.ContactErasableID)).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, gone)
	assert.False(t, eventExists(t, env, fixtures.EventErasableCustomID))
	sent := eventByID(t, env, fixtures.EventErasableSentID)
	assert.Nil(t, sent.ContactID)
	assert.Nil(t, sent.Email)
	unsub, err := env.DB.Unsubscribe.Get(ctx, 300)
	require.NoError(t, err)
	assert.Equal(t, fixtures.ContactErasableEmail, unsub.Destination)
	assert.Nil(t, unsub.ContactID)
	m, err := env.DB.OutboundMessage.Get(ctx, fixtures.OutboundMessageErasableTransactionalID)
	require.NoError(t, err)
	assert.Empty(t, m.Destination)
	b, err := env.DB.OutboundMessage.Get(ctx, fixtures.OutboundMessageBystanderID)
	require.NoError(t, err)
	assert.Equal(t, "ben@initech.test", b.Destination)
}

func TestExternalContactsEraseByEmailAnonymizesContactlessTransactionalSends(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	res, err := initechClient(t, env, "contacts:erase").ContactsEraseBy(ctx, externalapi.ContactsEraseByParams{
		Email: externalapi.NewOptEmailAddress(fixtures.OutboundMessageStrangerDestination),
	})
	require.NoError(t, err)
	require.IsTypef(t, &externalapi.ContactsEraseByNoContent{}, res, "got %T", res)

	m, err := env.DB.OutboundMessage.Get(ctx, fixtures.OutboundMessageStrangerID)
	require.NoError(t, err)
	assert.Empty(t, m.Destination)
	assert.Equal(t, outboundmessage.StatusSent, m.Status, "the row stays for the totals")
	other, err := env.DB.OutboundMessage.Get(ctx, fixtures.OutboundMessageBystanderID)
	require.NoError(t, err)
	assert.Equal(t, "ben@initech.test", other.Destination)
}

func TestExternalContactsEraseByVisitorWithoutContact(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	res, err := initechClient(t, env, "contacts:erase").ContactsEraseBy(ctx, externalapi.ContactsEraseByParams{
		VisitorId: externalapi.NewOptString("vis-anonymous-301"),
	})
	require.NoError(t, err)
	require.IsTypef(t, &externalapi.ContactsEraseByNoContent{}, res, "got %T", res)

	n, err := env.DB.Visitor.Query().Where(visitor.ID(fixtures.VisitorAnonymousID)).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, n)
	assert.False(t, eventExists(t, env, fixtures.EventAnonymousVisitorID))
	n, err = env.DB.Visitor.Query().Where(visitor.ID(300)).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "a Contact's device is untouched")
}

func TestExternalContactsEraseByNeedsExactlyOneKnownIdentifier(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := initechClient(t, env, "contacts:erase")

	none, err := c.ContactsEraseBy(ctx, externalapi.ContactsEraseByParams{})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactsEraseByBadRequest{}, none)

	both, err := c.ContactsEraseBy(ctx, externalapi.ContactsEraseByParams{
		Email:     externalapi.NewOptEmailAddress(fixtures.ContactErasableEmail),
		VisitorId: externalapi.NewOptString("vis-anonymous-301"),
	})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactsEraseByBadRequest{}, both)

	missing, err := c.ContactsEraseBy(ctx, externalapi.ContactsEraseByParams{
		Email: externalapi.NewOptEmailAddress("nobody@initech.test"),
	})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactsEraseByNotFound{}, missing)

	denied, err := initechClient(t, env, "contacts:write").ContactsEraseBy(ctx, externalapi.ContactsEraseByParams{
		Email: externalapi.NewOptEmailAddress(fixtures.ContactErasableEmail),
	})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactsEraseByForbidden{}, denied)

	cross, err := env.ExternalScoped(t, "contacts:erase").ContactsEraseBy(ctx, externalapi.ContactsEraseByParams{
		Email: externalapi.NewOptEmailAddress(fixtures.ContactErasableEmail),
	})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactsEraseByNotFound{}, cross)
}

// A visitor bound to a Contact erases that Contact, by the same rules as by id.
func TestExternalContactsEraseByVisitorOfAContactErasesTheContact(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	res, err := initechClient(t, env, "contacts:erase").ContactsEraseBy(ctx, externalapi.ContactsEraseByParams{
		VisitorId: externalapi.NewOptString("vis-erasable-laptop"),
	})
	require.NoError(t, err)
	require.IsTypef(t, &externalapi.ContactsEraseByNoContent{}, res, "got %T", res)
	gone, err := env.DB.Contact.Query().Where(contact.ID(fixtures.ContactErasableID)).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, gone)
}
