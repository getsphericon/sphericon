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

// The console cancels a scheduled Broadcast's schedule: nothing is sent, the Broadcast returns to
// draft with no audience, and scheduling it again takes the audience at that moment.
func TestSiteBroadcastUnscheduleReturnsAScheduledBroadcastToDraft(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	params := siteapi.SiteBroadcastsUnscheduleParams{
		WorkspaceId: fixtures.AcmePublicID,
		ID:          fixtures.BroadcastScheduledID,
	}

	b, err := c.SiteBroadcastsUnschedule(ctx, params)
	require.NoError(t, err)

	assert.Equal(t, siteapi.BroadcastStatusDraft, b.Status)
	assert.False(t, b.ScheduledAt.Set, "the schedule is gone")
	assert.False(t, b.Recipients.Set, "an unscheduled draft has taken no audience yet")

	scheduled, err := c.SiteBroadcastsSchedule(ctx, &siteapi.ScheduleBroadcastInput{
		Revision:    b.Revision,
		ScheduledAt: siteapi.Timestamp(time.Now().Add(time.Hour)),
	}, siteapi.SiteBroadcastsScheduleParams{WorkspaceId: fixtures.AcmePublicID, ID: params.ID})
	require.NoError(t, err)

	assert.Equal(t, siteapi.BroadcastStatusScheduled, scheduled.Status, "unscheduling is undone by scheduling again")
	assert.True(t, scheduled.Recipients.Set, "a schedule answers the audience it would take")
}

// A Broadcast with no schedule has none to cancel: the draft is refused and keeps its state.
func TestSiteBroadcastUnscheduleRefusesADraft(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	_, err := c.SiteBroadcastsUnschedule(ctx, siteapi.SiteBroadcastsUnscheduleParams{
		WorkspaceId: fixtures.AcmePublicID,
		ID:          fixtures.BroadcastDraftID,
	})
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity, "nothing to unschedule on a draft")

	assert.Equal(t, broadcast.StatusDraft, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).Status)
}
