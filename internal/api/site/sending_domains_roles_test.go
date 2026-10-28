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

// A Sending domain decides which domain mail is sent as and which host serves tracking
// links, so every write needs an Owner or an Admin; reading one stays open to a member.
func TestSiteSendingDomainsWritesNeedAnOwnerOrAdmin(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)

	created, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteSendingDomainsCreate(ctx,
		&siteapi.SiteCreateSendingDomainInput{Domain: "roles.acme.test"},
		siteapi.SiteSendingDomainsCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	id := created.(*siteapi.SiteSendingDomainResource).ID

	member := env.SiteActor(t, fixtures.MemberMaryEmail)

	made, err := member.SiteSendingDomainsCreate(ctx, &siteapi.SiteCreateSendingDomainInput{Domain: "attacker.example"},
		siteapi.SiteSendingDomainsCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	assert.IsType(t, new(siteapi.SiteSendingDomainsCreateForbidden), made)

	archived, err := member.SiteSendingDomainsArchive(ctx, siteapi.SiteSendingDomainsArchiveParams{WorkspaceId: workspaceID, ID: id})
	require.NoError(t, err)
	assert.IsType(t, new(siteapi.SiteSendingDomainsArchiveForbidden), archived)

	unarchived, err := member.SiteSendingDomainsUnarchive(ctx, siteapi.SiteSendingDomainsUnarchiveParams{WorkspaceId: workspaceID, ID: id})
	require.NoError(t, err)
	assert.IsType(t, new(siteapi.SiteSendingDomainsUnarchiveForbidden), unarchived)

	verified, err := member.SiteSendingDomainsVerify(ctx, siteapi.SiteSendingDomainsVerifyParams{WorkspaceId: workspaceID, ID: id})
	require.NoError(t, err)
	assert.IsType(t, new(siteapi.SiteSendingDomainsVerifyForbidden), verified)

	// Reading stays open to a member.
	got, err := member.SiteSendingDomainsGet(ctx, siteapi.SiteSendingDomainsGetParams{WorkspaceId: workspaceID, ID: id})
	require.NoError(t, err)
	assert.IsType(t, new(siteapi.SiteSendingDomainResource), got)

	// An admin manages them.
	ok, err := env.SiteActor(t, fixtures.AdminAliceEmail).SiteSendingDomainsVerify(ctx,
		siteapi.SiteSendingDomainsVerifyParams{WorkspaceId: workspaceID, ID: id})
	require.NoError(t, err)
	assert.IsType(t, new(siteapi.SiteSendingDomainResource), ok)
}
