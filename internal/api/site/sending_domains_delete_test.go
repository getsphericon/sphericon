package site_test

import (
	"context"
	"net/http"
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
	list, err := c.SiteSendingDomainsList(context.Background(), siteapi.SiteSendingDomainsListParams{
		WorkspaceId: fixtures.AcmePublicID,
		PageSize:    siteapi.NewOptInt32(100),
	})
	require.NoError(t, err)
	return lo.Map(list.Items, func(d siteapi.SiteSendingDomainResource, _ int) int64 { return int64(d.ID) })
}

func deleteSendingDomain(t *testing.T, c *siteapi.Client, id int64) error {
	t.Helper()
	return deleteSendingDomainOf(t, c, fixtures.AcmePublicID, id)
}

// deleteSendingDomainOf deletes the Sending domain id of the Workspace as c.
func deleteSendingDomainOf(t *testing.T, c *siteapi.Client, ws siteapi.WorkspaceId, id int64) error {
	t.Helper()
	return c.SiteSendingDomainsDelete(t.Context(), siteapi.SiteSendingDomainsDeleteParams{WorkspaceId: ws, ID: idStr(id)})
}

// A deleted Sending domain leaves the list for good and can still be read by id, marked deleted.
// Deleting it again changes nothing.
func TestSiteSendingDomainsDelete(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	id := int64(fixtures.SendingDomainUnverifiedID)

	assert.NotContains(t, listSendingDomains(t, c), int64(fixtures.SendingDomainArchivedID), "a deleted fixture row is hidden")

	require.NoError(t, deleteSendingDomain(t, c, id))

	assert.NotContains(t, listSendingDomains(t, c), id)
	opened, err := c.SiteSendingDomainsGet(ctx, siteapi.SiteSendingDomainsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(id)})
	require.NoError(t, err)
	assert.True(t, opened.DiscardedAt.IsSet())

	assert.NoError(t, deleteSendingDomain(t, c, id), "deleting it again changes nothing")
}

// A domain the Workspace deleted can be added again: a deleted row does not hold its name.
func TestSiteSendingDomainsDeletedNameIsReusable(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	require.NoError(t, deleteSendingDomain(t, c, fixtures.SendingDomainUnverifiedID))

	_, err := c.SiteSendingDomainsCreate(ctx, &siteapi.CreateSendingDomainInput{Domain: fixtures.SendingDomainUnverifiedDomain},
		siteapi.SiteSendingDomainsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err, "a deleted name is free again")
}

// A deleted Sending domain is never verified again (ADR 0100): Verify answers 404 and runs no check.
func TestSiteSendingDomainsDeletedOneIsNotVerified(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	id := idStr(fixtures.SendingDomainArchivedID)

	_, err := c.SiteSendingDomainsVerify(ctx, siteapi.SiteSendingDomainsVerifyParams{WorkspaceId: fixtures.AcmePublicID, ID: id})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)

	row, err := c.SiteSendingDomainsGet(ctx, siteapi.SiteSendingDomainsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: id})
	require.NoError(t, err)
	assert.False(t, row.LastCheckedAt.Set && !row.LastCheckedAt.Null, "no check ran")
}

// Deleting a domain that live work sends from is refused with 409 naming that work, and the
// domain stays kept.
func TestSiteSendingDomainsDeleteRefusedWhileLiveWorkSendsFromIt(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	refused := testhelper.SiteRefusal(t, deleteSendingDomain(t, c, fixtures.SendingDomainLaunchID), http.StatusConflict)
	assert.Equal(t, map[siteapi.UsedByKind][]string{siteapi.UsedByKindBroadcast: {fixtures.BroadcastScheduledFromLaunchDomainName}}, usedBy(refused))
	assert.Contains(t, listSendingDomains(t, c), int64(fixtures.SendingDomainLaunchID))
}
