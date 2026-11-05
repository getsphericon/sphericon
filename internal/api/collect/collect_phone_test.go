package collect_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/go-faster/jx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/visitor"
	collectapi "github.com/getsphericon/sphericon/gen/collect"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// ADR 0075: Identify with a local number resolves the Contact that stores it in E.164 instead of
// making a second one; the country comes from the traits or from the Contact another key finds.
func TestCollectIdentifyWithALocalPhoneResolvesTheContactStoredInE164(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.CollectAcme(t)
	ctx := context.Background()
	identify := func(in collectapi.CollectIdentifyInput) {
		t.Helper()
		_, err := c.CollectIdentifyCreate(ctx, &in)
		require.NoError(t, err)
	}
	// ContactTexter's phone, +14155550810, written as a US local number.
	identify(collectapi.CollectIdentifyInput{
		VisitorId: "dev-phone-2",
		Phone:     collectapi.NewOptNilString("(415) 555-0810"),
		Traits:    collectapi.NewOptNilJsonObject(collectapi.JsonObject{"country": jx.Raw(`"US"`)}),
	})

	n, err := env.DB.Contact.Query().Where(contact.WorkspaceID(fixtures.AcmeID), contact.PhoneContains("5550810")).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "one number in two formats is one Contact")
	v, err := env.DB.Visitor.Query().Where(visitor.WorkspaceID(fixtures.AcmeID), visitor.VisitorID("dev-phone-2")).Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, v.ContactID)
	assert.Equal(t, int64(fixtures.ContactTexterID), *v.ContactID)
}

func TestCollectIdentifyReadsALocalPhoneWithTheCountryOfTheContactItFinds(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.CollectAcme(t)
	ctx := context.Background()
	_, err := env.DB.Contact.UpdateOneID(fixtures.ContactAliceID).SetCountry("DE").Save(ctx)
	require.NoError(t, err)

	_, err = c.CollectIdentifyCreate(ctx, &collectapi.CollectIdentifyInput{
		VisitorId: "dev-alice",
		Email:     collectapi.NewOptNilEmailAddress(fixtures.ContactAliceEmail),
		Phone:     collectapi.NewOptNilString("030 1234567"),
	})
	require.NoError(t, err)

	alice, err := env.DB.Contact.Get(ctx, fixtures.ContactAliceID)
	require.NoError(t, err)
	require.NotNil(t, alice.Phone)
	assert.Equal(t, "+49301234567", *alice.Phone)
}

func TestCollectIdentifyRefusesALocalPhoneWithoutACountry(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	_, err := env.CollectAcme(t).CollectIdentifyCreate(ctx, &collectapi.CollectIdentifyInput{
		VisitorId: "dev-local",
		Phone:     collectapi.NewOptNilString("415 555 0107"),
	})
	testhelper.CollectRefusal(t, err, http.StatusUnprocessableEntity)

	n, err := env.DB.Contact.Query().Where(contact.WorkspaceID(fixtures.AcmeID), contact.PhoneContains("5550107")).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, n)
}
