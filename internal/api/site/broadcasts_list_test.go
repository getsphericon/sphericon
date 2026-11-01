package site_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func listedBroadcastIDs(t *testing.T, c *siteapi.Client, params siteapi.SiteBroadcastsListParams) []siteapi.EntityId {
	t.Helper()
	params.WorkspaceId = fixtures.AcmePublicID
	res, err := c.SiteBroadcastsList(t.Context(), params)
	require.NoError(t, err)
	list, ok := res.(*siteapi.SiteBroadcastsListOK)
	require.Truef(t, ok, "got %T", res)
	return lo.Map(list.Items, func(b siteapi.SiteBroadcastResource, _ int) siteapi.EntityId { return b.ID })
}

// Fixtures: Acme's Broadcasts scheduled in 3, 4 and 5 days, and one sending with no schedule.
func TestTheListNarrowsToStatusesAndPutsTheSoonestScheduleFirst(t *testing.T) {
	env := testhelper.Setup(t)

	ids := listedBroadcastIDs(t, env.SiteActor(t, fixtures.MemberMaryEmail), siteapi.SiteBroadcastsListParams{
		Status: []siteapi.SiteBroadcastStatus{siteapi.SiteBroadcastStatusScheduled, siteapi.SiteBroadcastStatusSending},
		Order:  siteapi.NewOptSiteBroadcastsOrder(siteapi.SiteBroadcastsOrderScheduledSoonest),
	})

	assert.Equal(t, []siteapi.EntityId{
		fixtures.BroadcastScheduledID, fixtures.BroadcastScheduledRenewalID,
		fixtures.BroadcastScheduledFromLaunchDomainID, fixtures.BroadcastSendingID,
	}, ids)
}

// Fixtures: Acme's Broadcasts sent 1, 3 and 9 days ago are its last three.
func TestTheListPutsTheLastSentFirst(t *testing.T) {
	env := testhelper.Setup(t)

	ids := listedBroadcastIDs(t, env.SiteActor(t, fixtures.MemberMaryEmail), siteapi.SiteBroadcastsListParams{
		Status:   []siteapi.SiteBroadcastStatus{siteapi.SiteBroadcastStatusSent},
		Order:    siteapi.NewOptSiteBroadcastsOrder(siteapi.SiteBroadcastsOrderSentLatest),
		PageSize: siteapi.NewOptInt32(3),
	})

	assert.Equal(t, []siteapi.EntityId{fixtures.BroadcastSentUntrackedID, fixtures.BroadcastSentID, fixtures.BroadcastSentNineDaysAgoID}, ids)
}
