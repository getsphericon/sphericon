package site_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
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
	params := siteapi.SiteContactsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: 1}

	res, err := c.SiteContactsUpdate(ctx, &siteapi.SiteUpdateContactInput{FirstName: nullString()}, params)
	require.NoError(t, err)
	got, ok := res.(*siteapi.SiteContactResource)
	require.Truef(t, ok, "got %T", res)
	assert.Empty(t, got.FirstName.Or(""), "first name cleared by explicit null")
	assert.Equal(t, "Smith", got.LastName.Or(""), "absent last name kept")
	assert.Equal(t, "alice@example.com", string(got.Email.Or("")), "absent email kept")

	// The clear is persisted, not just echoed.
	fetched, err := c.SiteContactsGet(ctx, siteapi.SiteContactsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: 1})
	require.NoError(t, err)
	fetchedRes, ok := fetched.(*siteapi.SiteContactResource)
	require.Truef(t, ok, "got %T", fetched)
	assert.Empty(t, fetchedRes.FirstName.Or(""))
}

// Custom fields are cleared as a whole by an explicit null. Fixture contact 100 has some.
func TestSiteContactsUpdateNullClearsCustomFields(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	params := siteapi.SiteContactsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: 100}

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

// The list narrows by a case-insensitive part of the stored email, and never reaches
// another Workspace's contact (ContactGlobex, pat@globex.test, belongs to Globex).
func TestSiteContactsListFiltersByEmail(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	emails := func(email string) []string {
		res, err := c.SiteContactsList(ctx, siteapi.SiteContactsListParams{
			WorkspaceId: fixtures.AcmePublicID,
			Email:       siteapi.NewOptString(email),
		})
		require.NoError(t, err)
		listed, ok := res.(*siteapi.SiteContactsListOK)
		require.Truef(t, ok, "got %T", res)
		out := make([]string, 0, len(listed.Items))
		for _, item := range listed.Items {
			out = append(out, string(item.Email.Or("")))
		}
		return out
	}

	assert.Equal(t, []string{"alice@example.com"}, emails("ALICE@Example"), "a hit, whatever the case")
	assert.Equal(t, []string{"bob@example.com"}, emails("bob@"), "a part of the address")
	assert.Empty(t, emails("nobody"), "a miss")
	assert.Empty(t, emails("pat@globex.test"), "another Workspace's contact is never returned")
}

// Tags ride on the contact: create gives them, an update with `tags` replaces the set, an
// absent key keeps it, and every read shows them ordered by name.
func TestSiteContactsCarryTheirTags(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	res, err := c.SiteContactsCreate(ctx, &siteapi.SiteCreateContactInput{
		Email: siteapi.NewOptNilEmailAddress("tagged@example.com"),
		Tags:  []string{"zeta", "alpha"},
	}, siteapi.SiteContactsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	created, ok := res.(*siteapi.SiteContactResource)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, []string{"alpha", "zeta"}, created.Tags)

	params := siteapi.SiteContactsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: created.ID}
	updated, err := c.SiteContactsUpdate(ctx, &siteapi.SiteUpdateContactInput{FirstName: nullString()}, params)
	require.NoError(t, err)
	kept, ok := updated.(*siteapi.SiteContactResource)
	require.Truef(t, ok, "got %T", updated)
	assert.Equal(t, []string{"alpha", "zeta"}, kept.Tags, "an absent tags key keeps the set")

	updated, err = c.SiteContactsUpdate(ctx, &siteapi.SiteUpdateContactInput{Tags: []string{"beta"}}, params)
	require.NoError(t, err)
	replaced, ok := updated.(*siteapi.SiteContactResource)
	require.Truef(t, ok, "got %T", updated)
	assert.Equal(t, []string{"beta"}, replaced.Tags)

	fetched, err := c.SiteContactsGet(ctx, siteapi.SiteContactsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: created.ID})
	require.NoError(t, err)
	got, ok := fetched.(*siteapi.SiteContactResource)
	require.Truef(t, ok, "got %T", fetched)
	assert.Equal(t, []string{"beta"}, got.Tags)
}
