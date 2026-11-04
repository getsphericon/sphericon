package site_test

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func listSendingDomains(t *testing.T, c *siteapi.Client) []int64 {
	t.Helper()
	res, err := c.SiteSendingDomainsList(context.Background(), siteapi.SiteSendingDomainsListParams{
		WorkspaceId: fixtures.AcmePublicID,
		PageSize:    siteapi.NewOptInt32(100),
	})
	require.NoError(t, err)
	list, ok := res.(*siteapi.SiteSendingDomainsListOK)
	require.Truef(t, ok, "got %T", res)
	return lo.Map(list.Items, func(d siteapi.SiteSendingDomainResource, _ int) int64 { return int64(d.ID) })
}

func deleteSendingDomain(t *testing.T, c *siteapi.Client, id int64) siteapi.SiteSendingDomainsDeleteRes {
	t.Helper()
	res, err := c.SiteSendingDomainsDelete(context.Background(), siteapi.SiteSendingDomainsDeleteParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(id)})
	require.NoError(t, err)
	return res
}

// A deleted Sending domain leaves the list for good and can still be read by id, marked deleted.
// Deleting it again changes nothing.
func TestSiteSendingDomainsDelete(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	id := int64(fixtures.SendingDomainUnverifiedID)

	assert.NotContains(t, listSendingDomains(t, c), int64(fixtures.SendingDomainArchivedID), "a deleted fixture row is hidden")

	assert.IsType(t, &siteapi.SiteSendingDomainsDeleteNoContent{}, deleteSendingDomain(t, c, id))

	assert.NotContains(t, listSendingDomains(t, c), id)
	got, err := c.SiteSendingDomainsGet(ctx, siteapi.SiteSendingDomainsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(id)})
	require.NoError(t, err)
	opened, ok := got.(*siteapi.SiteSendingDomainResource)
	require.Truef(t, ok, "a deleted domain can still be read by id, got %T", got)
	assert.True(t, opened.DeletedAt.IsSet())

	assert.IsType(t, &siteapi.SiteSendingDomainsDeleteNoContent{}, deleteSendingDomain(t, c, id), "deleting it again changes nothing")
}

// A domain the Workspace deleted can be added again: a deleted row does not hold its name.
func TestSiteSendingDomainsDeletedNameIsReusable(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	require.IsType(t, &siteapi.SiteSendingDomainsDeleteNoContent{}, deleteSendingDomain(t, c, fixtures.SendingDomainUnverifiedID))

	again, err := c.SiteSendingDomainsCreate(ctx, &siteapi.CreateSendingDomainInput{Domain: fixtures.SendingDomainUnverifiedDomain},
		siteapi.SiteSendingDomainsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSendingDomainResource{}, again, "a deleted name is free again")
}

// Deleting a domain that live work sends from is refused with 409 naming that work, and the
// domain stays kept.
func TestSiteSendingDomainsDeleteRefusedWhileLiveWorkSendsFromIt(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	refused, ok := deleteSendingDomain(t, c, fixtures.SendingDomainLaunchID).(*siteapi.InUseProblemDetails)
	require.True(t, ok)
	assert.Equal(t, map[siteapi.UsedByKind][]string{siteapi.UsedByKindBroadcast: {fixtures.BroadcastScheduledFromLaunchDomainName}}, usedBy(refused))
	assert.Contains(t, listSendingDomains(t, c), int64(fixtures.SendingDomainLaunchID))
}
