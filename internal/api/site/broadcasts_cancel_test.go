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

// The console cancels a sending Broadcast (ADR 0150): the answer names how many were left unsent,
// its figures count them cancelled with none pending, and the list finds it by its new status.
func TestSiteBroadcastCancelEndsASendingBroadcast(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	env.PlaceRecipient(ctx, fixtures.BroadcastSendingID, fixtures.ContactAliceID)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	b, err := c.SiteBroadcastsCancel(ctx, siteapi.SiteBroadcastsCancelParams{WorkspaceId: fixtures.AcmePublicID, ID: fixtures.BroadcastSendingID})
	require.NoError(t, err)
	listed, err := c.SiteBroadcastsList(ctx, siteapi.SiteBroadcastsListParams{
		WorkspaceId: fixtures.AcmePublicID,
		Status:      []siteapi.BroadcastStatus{siteapi.BroadcastStatusCancelled},
	})
	require.NoError(t, err)

	assert.Equal(t, siteapi.BroadcastStatusCancelled, b.Status)
	assert.Equal(t, siteapi.NewOptInt32(1), b.Unsent)
	assert.EqualValues(t, 1, b.Stats.CancelledCount)
	assert.Zero(t, b.Stats.PendingCount)
	ids := lo.Map(listed.Items, func(b siteapi.SiteBroadcastResource, _ int) int64 { return int64(b.ID) })
	assert.Equal(t, []int64{fixtures.BroadcastSendingID}, ids)
}
