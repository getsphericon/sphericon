package site_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/getsphericon/sphericon/ent/sendingdomain"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSiteSendingDomainsCRUD(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)

	// Invalid domain is rejected.
	bad, err := c.SiteSendingDomainsCreate(ctx, &siteapi.SiteCreateSendingDomainInput{Domain: "not a domain"},
		siteapi.SiteSendingDomainsCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSendingDomainsCreateUnprocessableEntity{}, bad)

	// Create mints a keypair and returns the DNS records to publish.
	created, err := c.SiteSendingDomainsCreate(ctx, &siteapi.SiteCreateSendingDomainInput{Domain: "Marketing.Example.COM"},
		siteapi.SiteSendingDomainsCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	res, ok := created.(*siteapi.SiteSendingDomainResource)
	require.Truef(t, ok, "got %T", created)
	assert.Equal(t, "marketing.example.com", res.Domain, "domain is normalized")
	assert.Equal(t, "sphericon", res.DkimSelector, "selector defaults to sphericon")
	assert.False(t, res.Verified, "a new domain starts unverified")
	assert.Equal(t, "sphericon._domainkey.marketing.example.com", res.DkimRecord.Host)
	assert.Contains(t, res.DkimRecord.Value, "v=DKIM1; k=rsa; p=")
	assert.Equal(t, siteapi.SiteDnsRecordTypeTXT, res.DkimRecord.Type)
	assert.NotEmpty(t, res.SpfRecord.Value)
	assert.Equal(t, "_dmarc.marketing.example.com", res.DmarcRecord.Host)

	// The private key is never exposed in any JSON field of the resource.
	raw, err := json.Marshal(res)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "PRIVATE KEY")

	// ...but it is persisted encrypted (never the plaintext PEM) and decryptable.
	id := int64(res.ID)
	row, err := env.DB.SendingDomain.Get(ctx, id)
	require.NoError(t, err)
	assert.NotContains(t, row.DkimPrivateKeyEncrypted, "PRIVATE KEY", "stored key must be ciphertext, not PEM")
	assert.NotEmpty(t, row.DkimPrivateKeyEncrypted)

	// Adding the same domain again conflicts.
	dup, err := c.SiteSendingDomainsCreate(ctx, &siteapi.SiteCreateSendingDomainInput{Domain: "marketing.example.com"},
		siteapi.SiteSendingDomainsCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSendingDomainsCreateConflict{}, dup)

	// Fetch it back by id.
	got, err := c.SiteSendingDomainsGet(ctx, siteapi.SiteSendingDomainsGetParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	gotRes, ok := got.(*siteapi.SiteSendingDomainResource)
	require.Truef(t, ok, "got %T", got)
	assert.Equal(t, res.ID, gotRes.ID)

	// Verify enqueues the DKIM re-check (inline: NXDOMAIN stub → stays unverified).
	ver, err := c.SiteSendingDomainsVerify(ctx, siteapi.SiteSendingDomainsVerifyParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSendingDomainsVerifyNoContent{}, ver)

	// Delete; a fetch by id then resolves to 404.
	del, err := c.SiteSendingDomainsDelete(ctx, siteapi.SiteSendingDomainsDeleteParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSendingDomainsDeleteNoContent{}, del)

	gone, err := c.SiteSendingDomainsGet(ctx, siteapi.SiteSendingDomainsGetParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSendingDomainsGetNotFound{}, gone)
}

func TestSiteSendingDomainsListScopedToWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	list, err := c.SiteSendingDomainsList(ctx, siteapi.SiteSendingDomainsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	page, ok := list.(*siteapi.SiteSendingDomainsListOK)
	require.Truef(t, ok, "got %T", list)

	// Count matches the acme fixtures dynamically (don't hardcode dataset size).
	want, err := env.DB.SendingDomain.Query().Where(sendingdomain.WorkspaceID(fixtures.AcmeID)).Count(ctx)
	require.NoError(t, err)
	assert.Len(t, page.Items, want)
	for _, item := range page.Items {
		assert.NotEmpty(t, item.Domain)
		assert.Equal(t, "sphericon._domainkey."+item.Domain, item.DkimRecord.Host)
	}
}

func TestSiteSendingDomainsRequireMembership(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	// A real, authenticated user with no membership on acme must get 404 — never
	// another tenant's sending domains.
	c := env.SiteActor(t, fixtures.OutsiderOscarEmail)
	res, err := c.SiteSendingDomainsList(ctx, siteapi.SiteSendingDomainsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSendingDomainsListNotFound{}, res)
}
