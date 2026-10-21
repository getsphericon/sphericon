package site_test

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/sendingdomain"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func listSendingDomains(t *testing.T, c *siteapi.Client, archived bool) []int64 {
	t.Helper()
	res, err := c.SiteSendingDomainsList(context.Background(), siteapi.SiteSendingDomainsListParams{
		WorkspaceId: fixtures.AcmePublicID,
		PageSize:    siteapi.NewOptInt32(100),
		Archived:    siteapi.NewOptBool(archived),
	})
	require.NoError(t, err)
	list, ok := res.(*siteapi.SiteSendingDomainsListOK)
	require.Truef(t, ok, "got %T", res)
	return lo.Map(list.Items, func(d siteapi.SiteSendingDomainResource, _ int) int64 { return int64(d.ID) })
}

func archiveSendingDomain(t *testing.T, c *siteapi.Client, id int64) siteapi.SiteSendingDomainsArchiveRes {
	t.Helper()
	res, err := c.SiteSendingDomainsArchive(context.Background(), siteapi.SiteSendingDomainsArchiveParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(id)})
	require.NoError(t, err)
	return res
}

func unarchiveSendingDomain(t *testing.T, c *siteapi.Client, id int64) siteapi.SiteSendingDomainsUnarchiveRes {
	t.Helper()
	res, err := c.SiteSendingDomainsUnarchive(context.Background(), siteapi.SiteSendingDomainsUnarchiveParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(id)})
	require.NoError(t, err)
	return res
}

// An archived Sending domain leaves the default list, shows with archived=true, can still be
// opened, and comes back when restored.
func TestSiteSendingDomainsArchiveAndRestore(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	id := int64(fixtures.SendingDomainUnverifiedID)

	assert.NotContains(t, listSendingDomains(t, c, false), int64(fixtures.SendingDomainArchivedID), "an archived fixture row is hidden")
	assert.Equal(t, []int64{fixtures.SendingDomainArchivedID}, listSendingDomains(t, c, true))

	out := archiveSendingDomain(t, c, id)
	archived, ok := out.(*siteapi.SiteSendingDomainResource)
	require.Truef(t, ok, "got %T", out)
	assert.True(t, archived.ArchivedAt.IsSet())

	assert.NotContains(t, listSendingDomains(t, c, false), id)
	assert.Contains(t, listSendingDomains(t, c, true), id)
	got, err := c.SiteSendingDomainsGet(ctx, siteapi.SiteSendingDomainsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(id)})
	require.NoError(t, err)
	opened, ok := got.(*siteapi.SiteSendingDomainResource)
	require.Truef(t, ok, "an archived domain can still be opened, got %T", got)
	assert.True(t, opened.ArchivedAt.IsSet())

	back, ok := unarchiveSendingDomain(t, c, id).(*siteapi.SiteSendingDomainResource)
	require.Truef(t, ok, "got %T", back)
	assert.False(t, back.ArchivedAt.IsSet())
	assert.Contains(t, listSendingDomains(t, c, false), id)
	assert.NotContains(t, listSendingDomains(t, c, true), id)
}

// A domain the Workspace archived can be added again, and then the archived one cannot come back
// until that one is archived in turn: two kept rows never claim one name.
func TestSiteSendingDomainsArchivedNameIsReusableAndUnarchiveConflicts(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	id := int64(fixtures.SendingDomainUnverifiedID)
	require.IsType(t, &siteapi.SiteSendingDomainResource{}, archiveSendingDomain(t, c, id))

	again, err := c.SiteSendingDomainsCreate(ctx, &siteapi.SiteCreateSendingDomainInput{Domain: fixtures.SendingDomainUnverifiedDomain},
		siteapi.SiteSendingDomainsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	created, ok := again.(*siteapi.SiteSendingDomainResource)
	require.Truef(t, ok, "an archived name is free again, got %T", again)

	assert.IsType(t, &siteapi.SiteSendingDomainsUnarchiveConflict{}, unarchiveSendingDomain(t, c, id))
	assert.Contains(t, listSendingDomains(t, c, true), id, "a refused restore leaves the row archived")

	require.IsType(t, &siteapi.SiteSendingDomainResource{}, archiveSendingDomain(t, c, int64(created.ID)))
	assert.IsType(t, &siteapi.SiteSendingDomainResource{}, unarchiveSendingDomain(t, c, id))
}

// Restoring starts a new DNS check, and the Tracking domain starts over: the certificate and the
// serving state of the retired host are not restored.
func TestSiteSendingDomainsUnarchiveStartsOverFromPending(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	back, ok := unarchiveSendingDomain(t, c, fixtures.SendingDomainArchivedID).(*siteapi.SiteSendingDomainResource)
	require.Truef(t, ok, "got %T", back)
	assert.False(t, back.ArchivedAt.IsSet())

	row, err := env.DB.Scoped(fixtures.AcmeID).SendingDomain().Get(ctx, fixtures.SendingDomainArchivedID)
	require.NoError(t, err)
	assert.Nil(t, row.DiscardedAt)
	assert.NotNil(t, row.LastCheckedAt, "the DNS check ran")
	assert.Equal(t, sendingdomain.TrackingStatePending, row.TrackingState, "no serving state is restored")
}

// Archiving a domain that live work sends from is refused with 409 naming that work, and the
// domain stays kept.
func TestSiteSendingDomainsArchiveRefusedWhileLiveWorkSendsFromIt(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	refused, ok := archiveSendingDomain(t, c, fixtures.SendingDomainLaunchID).(*siteapi.InUseProblemDetails)
	require.True(t, ok)
	assert.Equal(t, map[siteapi.UsedByKind][]string{siteapi.UsedByKindBroadcast: {fixtures.BroadcastScheduledFromLaunchDomainName}}, usedBy(refused))
	assert.Contains(t, listSendingDomains(t, c, false), int64(fixtures.SendingDomainLaunchID))
}
