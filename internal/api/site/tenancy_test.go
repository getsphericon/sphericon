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

// Fixture user info@getsphericon.com owns workspace fixtures.AcmePublicID (id 1), which owns the three
// seeded contacts. The dashboard addresses contacts via /w/{workspaceID}/contacts.
func TestSiteContactsScopedToWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	// Contacts of the owned workspace are listed (the workspace has seeded
	// contacts; assert presence, not an exact count).
	list, err := c.SiteContactsList(ctx, siteapi.SiteContactsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	listed, ok := list.(*siteapi.SiteContactsListOK)
	require.Truef(t, ok, "got %T", list)
	assert.NotEmpty(t, listed.Items, "owned workspace returns its contacts")

	// Creating a contact scopes it to the workspace.
	created, err := c.SiteContactsCreate(ctx, &siteapi.SiteCreateContactInput{Email: siteapi.NewOptNilEmailAddress("site-new@example.com")}, siteapi.SiteContactsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteContactResource{}, created)

	// An unknown / non-owned workspace workspaceID resolves to 404, not a data leak.
	missing, err := c.SiteContactsList(ctx, siteapi.SiteContactsListParams{WorkspaceId: "NoSuchWs"})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteContactsListNotFound{}, missing)
}

func TestSiteWorkspacesList(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	got, err := c.SiteWorkspacesList(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, siteapi.WorkspaceId(fixtures.AcmePublicID), got[0].ID)
}

// Access is by Membership, not single ownership: a User with no Membership on a
// workspace can neither see it nor reach its scoped resources.
func TestSiteWorkspaceRequiresMembership(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	// A real, authenticated User who is a member of no workspace.
	c := env.SiteActor(t, fixtures.OutsiderOscarEmail)

	// Sees no workspaces at all.
	got, err := c.SiteWorkspacesList(ctx)
	require.NoError(t, err)
	assert.Empty(t, got)

	// And cannot reach acme's contacts — 404, not a data leak.
	missing, err := c.SiteContactsList(ctx, siteapi.SiteContactsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteContactsListNotFound{}, missing)
}

// A member of another tenant sees only their own workspace and cannot reach Acme.
func TestSiteWorkspaceIsolatedBetweenTenants(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.SiteActor(t, fixtures.OwnerJaneEmail)

	got, err := c.SiteWorkspacesList(ctx)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, siteapi.WorkspaceId(fixtures.GlobexPublicID), got[0].ID)

	missing, err := c.SiteContactsList(ctx, siteapi.SiteContactsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteContactsListNotFound{}, missing)
}
