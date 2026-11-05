package site_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/broadcast"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An edit's step may name a disabled Integration; publishing the Automation arms every send step,
// so it is a 409 while any step's Integration cannot send, though the other step's can.
func TestSiteAutomationPublishIsAConflictWhileAStepsIntegrationCannotSend(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)
	id := idStr(fixtures.AutomationHoldDemoID)
	_, err := c.SiteAutomationsUpdate(ctx, &siteapi.SiteUpdateAutomationInput{
		Steps: []siteapi.SiteAutomationStep{
			siteEmailStepVia(siteapi.OptNilEntityId{}),
			siteEmailStepVia(siteapi.NewOptNilEntityId(idStr(fixtures.IntegrationAcmeSesID))),
		},
	}, siteapi.SiteAutomationsUpdateParams{WorkspaceId: workspaceID, ID: id})
	require.NoError(t, err)

	_, err = c.SiteAutomationsPublish(ctx, siteapi.SiteAutomationsPublishParams{WorkspaceId: workspaceID, ID: id})

	conflict := testhelper.SiteRefusal(t, err, http.StatusConflict)
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
	_, err := c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{
		IntegrationId: siteapi.NewOptNilEntityId(idStr(fixtures.IntegrationAcmeSesID)),
	}, siteapi.SiteBroadcastsUpdateParams{WorkspaceId: workspaceID, ID: id})
	require.NoError(t, err)

	_, err = c.SiteBroadcastsSchedule(ctx, &siteapi.ScheduleBroadcastInput{
		ScheduledAt: siteapi.Timestamp(time.Now().Add(time.Hour)),
	}, siteapi.SiteBroadcastsScheduleParams{WorkspaceId: workspaceID, ID: id})

	conflict := testhelper.SiteRefusal(t, err, http.StatusConflict)
	assert.Contains(t, conflict.Detail.Or(""), "cannot send")
	status := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).Status
	assert.Equal(t, broadcast.StatusDraft, status)
}
