package site_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func listTrackingDomains(t *testing.T, c *siteapi.Client, ws siteapi.WorkspaceId) []siteapi.SiteTrackingDomainResource {
	t.Helper()
	list, err := c.SiteTrackingDomainsList(context.Background(), siteapi.SiteTrackingDomainsListParams{WorkspaceId: ws})
	require.NoError(t, err)
	return list
}

func setTrackingDomain(t *testing.T, c *siteapi.Client, ws siteapi.WorkspaceId, host string) (*siteapi.SiteTrackingDomainResource, error) {
	t.Helper()
	return c.SiteTrackingDomainsSet(context.Background(), &siteapi.SetTrackingDomainInput{Host: host},
		siteapi.SiteTrackingDomainsSetParams{WorkspaceId: ws})
}

// Any member reads the Workspace's Tracking domains, current first, each with its CNAME at the
// Instance's edge host.
func TestSiteTrackingDomainsListsThemCurrentFirstWithTheRecordToPublish(t *testing.T) {
	env := testhelper.Setup(t)

	list := listTrackingDomains(t, env.SiteActor(t, fixtures.OwnerJaneEmail), fixtures.GlobexPublicID)
	require.Len(t, list, 2)
	assert.Equal(t, fixtures.TrackingDomainGlobexHost, list[0].Host)
	assert.Equal(t, siteapi.SiteTrackingDomainRoleCurrent, list[0].Role)
	assert.Equal(t, siteapi.SiteTrackingStateDNSVerified, list[0].State)
	assert.False(t, list[0].ServedAt.IsSet())
	assert.Equal(t, siteapi.SiteTrackingDomainRoleReplacement, list[1].Role)

	rec := list[0].Record
	assert.Equal(t, siteapi.SiteCnameRecordTypeCNAME, rec.Type)
	assert.Equal(t, fixtures.TrackingDomainGlobexHost, rec.Host)
	assert.Equal(t, testhelper.TrackingCNAMETarget, rec.Target)

	member := listTrackingDomains(t, env.SiteActor(t, fixtures.MemberMaryEmail), fixtures.AcmePublicID)
	require.Len(t, member, 1, "a member reads them too")
}

// The current host does not serve, so a new one replaces it at once and starts pending.
func TestSiteTrackingDomainsSetReplacesAHostThatDoesNotServe(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	got, err := setTrackingDomain(t, c, fixtures.AcmePublicID, "Links.Acme.com")
	require.NoError(t, err)
	assert.Equal(t, "links.acme.com", got.Host)
	assert.Equal(t, siteapi.SiteTrackingDomainRoleCurrent, got.Role)
	assert.Equal(t, siteapi.SiteTrackingStatePending, got.State)

	list := listTrackingDomains(t, c, fixtures.AcmePublicID)
	require.Len(t, list, 1)
	assert.Equal(t, "links.acme.com", list[0].Host)
}

func TestSiteTrackingDomainsSetRefusesAnApexHost(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := setTrackingDomain(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.AcmePublicID, "acme.com")
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
}

// Removing a current host that never served deletes it and the replacement takes its place;
// cancelling that replacement leaves none.
func TestSiteTrackingDomainsRemovePromotesTheReplacementThenCancelsIt(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJaneEmail)
	ctx := context.Background()
	remove := func(id int64) error {
		return c.SiteTrackingDomainsRemove(ctx, siteapi.SiteTrackingDomainsRemoveParams{WorkspaceId: fixtures.GlobexPublicID, ID: siteapi.EntityId(id)})
	}

	require.NoError(t, remove(fixtures.TrackingDomainGlobexID))
	list := listTrackingDomains(t, c, fixtures.GlobexPublicID)
	require.Len(t, list, 1)
	assert.Equal(t, fixtures.TrackingDomainGlobexReplacementHost, list[0].Host)
	assert.Equal(t, siteapi.SiteTrackingDomainRoleCurrent, list[0].Role)

	require.NoError(t, remove(fixtures.TrackingDomainGlobexReplacementID))
	assert.Empty(t, listTrackingDomains(t, c, fixtures.GlobexPublicID))
}

// Setting, verifying and removing need tracking_domains:manage, which a member lacks.
func TestSiteTrackingDomainsWritesAreForOwnersAndAdmins(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.MemberMaryEmail)
	ctx := context.Background()
	ws := siteapi.WorkspaceId(fixtures.AcmePublicID)

	_, err := setTrackingDomain(t, c, ws, "links.acme.com")
	testhelper.SiteRefusal(t, err, http.StatusForbidden)
	_, err = c.SiteTrackingDomainsVerify(ctx, siteapi.SiteTrackingDomainsVerifyParams{WorkspaceId: ws, ID: fixtures.TrackingDomainAcmeID})
	testhelper.SiteRefusal(t, err, http.StatusForbidden)
	err = c.SiteTrackingDomainsRemove(ctx, siteapi.SiteTrackingDomainsRemoveParams{WorkspaceId: ws, ID: fixtures.TrackingDomainAcmeID})
	testhelper.SiteRefusal(t, err, http.StatusForbidden)
}

// Without a CNAME target the Instance offers no Tracking domains: the Workspace names no
// target and a host cannot be set.
func TestSiteTrackingDomainsAreOffWithoutATarget(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithConfig(func(c *config.Config) { c.TrackingCNAMETarget = "" }))
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	workspaces, err := c.SiteWorkspacesList(context.Background())
	require.NoError(t, err)
	require.Len(t, workspaces, 1)
	assert.False(t, workspaces[0].TrackingTarget.IsSet())
	_, err = setTrackingDomain(t, c, fixtures.AcmePublicID, "links.acme.com")
	testhelper.SiteRefusal(t, err, http.StatusNotFound)

	on := testhelper.Setup(t)
	workspaces, err = on.SiteActor(t, fixtures.OwnerJohnEmail).SiteWorkspacesList(context.Background())
	require.NoError(t, err)
	assert.Equal(t, testhelper.TrackingCNAMETarget, workspaces[0].TrackingTarget.Value)
}
