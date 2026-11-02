package site_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/holdreason"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A sending Broadcast reports progress, an ETA derived from its last scheduled time or its
// Workspace's Send cap, whichever is later, and which of the two limits it; a held one reports
// progress but no ETA and no limit (a hold is not a busy source); a draft has no progress at all.
func TestSiteBroadcastProgressAndETA(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	id := int64(fixtures.BroadcastDraftID)
	params := siteapi.SiteBroadcastsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(id)}
	get := func() siteapi.SiteBroadcastResource {
		got, err := c.SiteBroadcastsGet(ctx, params)
		require.NoError(t, err)
		res, ok := got.(*siteapi.SiteBroadcastResource)
		require.Truef(t, ok, "got %T", got)
		return *res
	}

	assert.False(t, get().Progress.IsSet(), "a draft has no progress")

	eta := time.Now().Add(90 * time.Minute).Truncate(time.Microsecond)
	require.NoError(t, s.Broadcast().UpdateOneID(id).SetStatus(broadcast.StatusSending).SetLastScheduledAt(eta).Exec(ctx))
	// Bob's recipient is decided by its Outbound message (ADR 0015); the other two are pending.
	decided := env.PlaceRecipient(ctx, id, fixtures.ContactBobID)
	env.PlaceRecipient(ctx, id, fixtures.ContactAliceID)
	env.PlaceRecipient(ctx, id, fixtures.ContactCarolID)
	require.NoError(t, s.OutboundMessage().Update().
		Where(outboundmessage.BroadcastRecipientID(decided)).SetStatus(outboundmessage.StatusSent).Exec(ctx))

	progress, ok := get().Progress.Get()
	require.True(t, ok)
	assert.Equal(t, int32(1), progress.ProcessedCount)
	assert.Equal(t, int32(2), progress.RemainingCount)
	got, ok := progress.EstimatedCompletionAt.Get()
	require.True(t, ok)
	assert.WithinDuration(t, eta, time.Time(got), time.Second, "timestamps are served to the second")
	assert.Equal(t, siteapi.NewNilSiteBroadcastLimit(siteapi.SiteBroadcastLimitProviderLimit), progress.LimitedBy, "the planner paced it by the Provider limit")

	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSendCap(1).ExecX(ctx)
	progress, ok = get().Progress.Get()
	require.True(t, ok)
	assert.Equal(t, siteapi.NewNilSiteBroadcastLimit(siteapi.SiteBroadcastLimitSendCap), progress.LimitedBy, "a Send cap of one a day outlasts the schedule")
	got, ok = progress.EstimatedCompletionAt.Get()
	require.True(t, ok)
	assert.WithinDuration(t, time.Now().Add(24*time.Hour), time.Time(got), time.Minute, "the second recipient waits a day")
	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).ClearSendCap().ExecX(ctx)

	list, err := c.SiteBroadcastsList(ctx, siteapi.SiteBroadcastsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	listed, ok := list.(*siteapi.SiteBroadcastsListOK)
	require.Truef(t, ok, "got %T", list)
	for _, item := range listed.Items {
		assert.False(t, item.Progress.IsSet(), "the list leaves progress to get and send")
	}

	require.NoError(t, s.Broadcast().UpdateOneID(id).SetHoldReason(holdreason.UnverifiedDomain).Exec(ctx))
	res := get()
	assert.Equal(t, siteapi.HoldReasonUnverifiedDomain, res.HoldReason.Value)
	progress, ok = res.Progress.Get()
	require.True(t, ok)
	assert.Equal(t, int32(2), progress.RemainingCount, "recipients stay intact under a hold")
	_, hasETA := progress.EstimatedCompletionAt.Get()
	assert.False(t, hasETA, "no ETA while held")
	assert.True(t, progress.LimitedBy.IsNull(), "a hold is not a limit")
}
