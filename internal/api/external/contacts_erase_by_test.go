package external_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contact"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An address names the Contact that holds it; what its Erasure leaves is the use case's.
func TestExternalContactsEraseByEmailErasesTheContact(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	err := initechClient(t, env, "contacts:erase").ContactsEraseBy(ctx, externalapi.ContactsEraseByParams{
		Email: externalapi.NewOptEmailAddress(fixtures.ContactErasableEmail),
	})
	require.NoError(t, err)

	gone, err := env.DB.Contact.Query().Where(contact.ID(fixtures.ContactErasableID)).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, gone)
}

func TestExternalContactsEraseByNeedsExactlyOneKnownIdentifier(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := initechClient(t, env, "contacts:erase")

	err := c.ContactsEraseBy(ctx, externalapi.ContactsEraseByParams{})
	testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)

	err = c.ContactsEraseBy(ctx, externalapi.ContactsEraseByParams{
		Email:     externalapi.NewOptEmailAddress(fixtures.ContactErasableEmail),
		VisitorId: externalapi.NewOptString("vis-anonymous-301"),
	})
	testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)

	err = c.ContactsEraseBy(ctx, externalapi.ContactsEraseByParams{
		Email: externalapi.NewOptEmailAddress("nobody@initech.test"),
	})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound)

	err = initechClient(t, env, "contacts:write").ContactsEraseBy(ctx, externalapi.ContactsEraseByParams{
		Email: externalapi.NewOptEmailAddress(fixtures.ContactErasableEmail),
	})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)

	err = env.ExternalScoped(t, apitokens.ScopeContactsErase).ContactsEraseBy(ctx, externalapi.ContactsEraseByParams{
		Email: externalapi.NewOptEmailAddress(fixtures.ContactErasableEmail),
	})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound)
}

// A visitor bound to a Contact erases that Contact, by the same rules as by id.
func TestExternalContactsEraseByVisitorOfAContactErasesTheContact(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	err := initechClient(t, env, "contacts:erase").ContactsEraseBy(ctx, externalapi.ContactsEraseByParams{
		VisitorId: externalapi.NewOptString("vis-erasable-laptop"),
	})
	require.NoError(t, err)
	gone, err := env.DB.Contact.Query().Where(contact.ID(fixtures.ContactErasableID)).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, gone)
}
