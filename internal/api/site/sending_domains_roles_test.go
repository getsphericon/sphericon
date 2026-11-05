package site_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A Sending domain decides which domain mail is sent as and which host serves tracking
// links, so every write needs an Owner or an Admin; reading one stays open to a member.
func TestSiteSendingDomainsWritesNeedAnOwnerOrAdmin(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)

	id := siteapi.EntityId(fixtures.SendingDomainUnverifiedID)

	member := env.SiteActor(t, fixtures.MemberMaryEmail)

	_, err := member.SiteSendingDomainsCreate(ctx, &siteapi.CreateSendingDomainInput{Domain: "attacker.example"},
		siteapi.SiteSendingDomainsCreateParams{WorkspaceId: workspaceID})
	testhelper.SiteRefusal(t, err, http.StatusForbidden)

	err = member.SiteSendingDomainsDelete(ctx, siteapi.SiteSendingDomainsDeleteParams{WorkspaceId: workspaceID, ID: id})
	testhelper.SiteRefusal(t, err, http.StatusForbidden)

	_, err = member.SiteSendingDomainsVerify(ctx, siteapi.SiteSendingDomainsVerifyParams{WorkspaceId: workspaceID, ID: id})
	testhelper.SiteRefusal(t, err, http.StatusForbidden)

	// Reading stays open to a member.
	_, err = member.SiteSendingDomainsGet(ctx, siteapi.SiteSendingDomainsGetParams{WorkspaceId: workspaceID, ID: id})
	require.NoError(t, err)

	// An admin manages them.
	_, err = env.SiteActor(t, fixtures.AdminAliceEmail).SiteSendingDomainsVerify(ctx,
		siteapi.SiteSendingDomainsVerifyParams{WorkspaceId: workspaceID, ID: id})
	require.NoError(t, err)
}
