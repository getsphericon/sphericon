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

// The site list returns the workspace's transactional sends, scoped to the
// workspace and never leaking another tenant's rows.
func TestSiteTransactionalEmailsList(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	// acme's own transactional sends come from the fixtures; Globex's is the cross-tenant negative case.

	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	res, err := c.SiteTransactionalEmailsList(ctx, siteapi.SiteTransactionalEmailsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)

	page, ok := res.(*siteapi.SiteTransactionalEmailsListOK)
	require.Truef(t, ok, "got %T", res)
	require.NotEmpty(t, page.Items, "acme has seeded transactional sends")
	for _, it := range page.Items {
		assert.NotEqual(t, fixtures.ContactGlobexEmail, it.Destination, "cross-workspace row leaked")
	}
}

// The list requires a valid JWT like the rest of the site API.
func TestSiteTransactionalEmailsRequireAuth(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteAnonymous(t)

	_, err := c.SiteTransactionalEmailsList(context.Background(), siteapi.SiteTransactionalEmailsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.Error(t, err)
}
