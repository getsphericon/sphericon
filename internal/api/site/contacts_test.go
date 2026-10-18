package site_test

import (
	"context"
	"testing"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func nullString() siteapi.OptNilString {
	var v siteapi.OptNilString
	v.SetToNull()
	return v
}

func nullEmail() siteapi.OptNilEmailAddress {
	var v siteapi.OptNilEmailAddress
	v.SetToNull()
	return v
}

// JSON Merge Patch on update: an explicit null clears an optional field and an
// absent key leaves it unchanged. Anchor contact 1 (alice@example.com) has an
// email, a first name and a last name.
func TestSiteContactsUpdateNullClearsAbsentKeeps(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	params := siteapi.SiteContactsUpdateParams{Slug: fixtures.AcmeSlug, ID: 1}

	res, err := c.SiteContactsUpdate(ctx, &siteapi.SiteUpdateContactInput{FirstName: nullString()}, params)
	require.NoError(t, err)
	got, ok := res.(*siteapi.SiteContactResource)
	require.Truef(t, ok, "got %T", res)
	assert.Empty(t, got.FirstName.Or(""), "first name cleared by explicit null")
	assert.Equal(t, "Smith", got.LastName.Or(""), "absent last name kept")
	assert.Equal(t, "alice@example.com", string(got.Email.Or("")), "absent email kept")

	// The clear is persisted, not just echoed.
	fetched, err := c.SiteContactsGet(ctx, siteapi.SiteContactsGetParams{Slug: fixtures.AcmeSlug, ID: 1})
	require.NoError(t, err)
	fetchedRes, ok := fetched.(*siteapi.SiteContactResource)
	require.Truef(t, ok, "got %T", fetched)
	assert.Empty(t, fetchedRes.FirstName.Or(""))
}

// An alias key (email) can be cleared with null. ADR 0002 keeps every alias key
// optional (anonymous Contacts have none), so clearing the last one is allowed.
// Fixture contact 2 (bob@example.com) has no other alias key.
func TestSiteContactsUpdateClearingLastAliasKeyIsAllowed(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	res, err := c.SiteContactsUpdate(ctx, &siteapi.SiteUpdateContactInput{Email: nullEmail()},
		siteapi.SiteContactsUpdateParams{Slug: fixtures.AcmeSlug, ID: 2})
	require.NoError(t, err)
	got, ok := res.(*siteapi.SiteContactResource)
	require.Truef(t, ok, "got %T", res)
	assert.Empty(t, got.Email.Or(""))
}

// Custom fields are cleared as a whole by an explicit null. Fixture contact 100 has some.
func TestSiteContactsUpdateNullClearsCustomFields(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	params := siteapi.SiteContactsUpdateParams{Slug: fixtures.AcmeSlug, ID: 100}

	var null siteapi.OptNilSiteUpdateContactInputCustomFields
	null.SetToNull()
	res, err := c.SiteContactsUpdate(ctx, &siteapi.SiteUpdateContactInput{CustomFields: null}, params)
	require.NoError(t, err)
	got, ok := res.(*siteapi.SiteContactResource)
	require.Truef(t, ok, "got %T", res)
	assert.Empty(t, got.CustomFields.Value)
	assert.Equal(t, "Liam", got.FirstName.Or(""), "absent first name kept")
}

// Site contacts require a valid JWT cookie (generated SecurityHandler). Without
// one the request is rejected; the typed client surfaces the 401 as an error.
func TestSiteContactsRequireAuth(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteAnonymous(t)

	_, err := c.SiteContactsList(context.Background(), siteapi.SiteContactsListParams{})
	require.Error(t, err)
}
