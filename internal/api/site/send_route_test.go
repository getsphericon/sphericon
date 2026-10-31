package site_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/broadcast"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An edit may name a disabled Integration; publishing the Automation arms its email step, so it is
// a 409 while the Integration cannot send.
func TestSiteAutomationPublishIsAConflictWhileItsIntegrationCannotSend(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)
	id := idStr(fixtures.AutomationHoldDemoID)
	edited, err := c.SiteAutomationsUpdate(ctx, &siteapi.SiteUpdateAutomationInput{
		IntegrationId: siteapi.NewOptNilEntityId(idStr(fixtures.IntegrationAcmeSesID)),
	}, siteapi.SiteAutomationsUpdateParams{WorkspaceId: workspaceID, ID: id})
	require.NoError(t, err)
	_, ok := edited.(*siteapi.SiteAutomationResource)
	require.Truef(t, ok, "the edit is saved, got %T", edited)

	res, err := c.SiteAutomationsPublish(ctx, siteapi.SiteAutomationsPublishParams{WorkspaceId: workspaceID, ID: id})

	require.NoError(t, err)
	conflict, ok := res.(*siteapi.SiteAutomationsPublishConflict)
	require.Truef(t, ok, "got %T", res)
	assert.Contains(t, conflict.Detail.Or(""), "cannot send")
}

// An edit may name a disabled Integration; scheduling the Broadcast arms its send, so it is a 409
// while the Integration cannot send, and the Broadcast stays a draft.
func TestSiteBroadcastScheduleIsAConflictWhileItsIntegrationCannotSend(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)
	id := idStr(fixtures.BroadcastDraftID)
	edited, err := c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{
		IntegrationId: siteapi.NewOptNilEntityId(idStr(fixtures.IntegrationAcmeSesID)),
	}, siteapi.SiteBroadcastsUpdateParams{WorkspaceId: workspaceID, ID: id})
	require.NoError(t, err)
	_, ok := edited.(*siteapi.SiteBroadcastResource)
	require.Truef(t, ok, "the edit is saved, got %T", edited)

	res, err := c.SiteBroadcastsSchedule(ctx, &siteapi.ScheduleBroadcastInput{
		ScheduledAt: siteapi.Timestamp(time.Now().Add(time.Hour)),
	}, siteapi.SiteBroadcastsScheduleParams{WorkspaceId: workspaceID, ID: id})

	require.NoError(t, err)
	conflict, ok := res.(*siteapi.SiteBroadcastsScheduleConflict)
	require.Truef(t, ok, "got %T", res)
	assert.Contains(t, conflict.Detail.Or(""), "cannot send")
	status := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).Status
	assert.Equal(t, broadcast.StatusDraft, status)
}
