package site_test

import (
	"context"
	"testing"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedTransactional(t *testing.T, db *ent.Client, ws int64, dest string, status outboundmessage.Status) {
	t.Helper()
	_, err := db.OutboundMessage.Create().
		SetWorkspaceID(ws).
		SetKind(outboundmessage.KindTransactional).
		SetIdempotencyKey("transactional:seed:" + dest).
		SetDestination(dest).
		SetTemplateID(fixtures.TemplateWelcomeID).
		SetStatus(status).
		Save(context.Background())
	require.NoError(t, err)
}

// The site list returns the workspace's transactional sends, scoped to the
// workspace and never leaking another tenant's rows.
func TestSiteTransactionalEmailsList(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	// acme's own transactional sends come from the fixtures. A second workspace's
	// row is seeded only to prove it is excluded (the cross-tenant negative case).
	seedTransactional(t, env.DB, fixtures.GlobexID, "leak@example.com", outboundmessage.StatusSent)

	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	res, err := c.SiteTransactionalEmailsList(ctx, siteapi.SiteTransactionalEmailsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)

	page, ok := res.(*siteapi.SiteTransactionalEmailsListOK)
	require.Truef(t, ok, "got %T", res)
	require.NotEmpty(t, page.Items, "acme has seeded transactional sends")
	for _, it := range page.Items {
		assert.NotEqual(t, "leak@example.com", it.Destination, "cross-workspace row leaked")
	}
}

// The list requires a valid JWT like the rest of the site API.
func TestSiteTransactionalEmailsRequireAuth(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteAnonymous(t)

	_, err := c.SiteTransactionalEmailsList(context.Background(), siteapi.SiteTransactionalEmailsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.Error(t, err)
}
