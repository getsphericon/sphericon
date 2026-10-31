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
	"github.com/getsphericon/sphericon/internal/suspension"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A sending Broadcast reports progress and an ETA derived from its last scheduled
// time; a held one reports progress but no ETA (a hold is not a busy source); a draft
// has no progress at all.
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
	assert.Equal(t, siteapi.SiteBroadcastLimitProviderLimit, progress.LimitedBy.Value, "the planner's pace is the Provider limit's")

	list, err := c.SiteBroadcastsList(ctx, siteapi.SiteBroadcastsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	listed, ok := list.(*siteapi.SiteBroadcastsListOK)
	require.Truef(t, ok, "got %T", list)
	for _, item := range listed.Items {
		assert.False(t, item.Progress.IsSet(), "the list leaves progress to get and send")
	}

	require.NoError(t, s.Broadcast().UpdateOneID(id).SetHoldReason("unverified_domain").Exec(ctx))
	res := get()
	assert.Equal(t, "unverified_domain", res.HoldReason.Value)
	progress, ok = res.Progress.Get()
	require.True(t, ok)
	assert.Equal(t, int32(2), progress.RemainingCount, "recipients stay intact under a hold")
	_, hasETA := progress.EstimatedCompletionAt.Get()
	assert.False(t, hasETA, "no ETA while held")
	assert.True(t, progress.LimitedBy.Null, "a hold blocks, nothing paces")
}

// A sending Broadcast beyond the Workspace's Send cap names the Send cap, with its estimate.
func TestSiteBroadcastProgressNamesTheSendCap(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	id := int64(fixtures.BroadcastDraftID)
	sendCap := 1
	_, err := suspension.SetSendCap(ctx, env.Bus, fixtures.AcmeID, suspension.StaffMember("op-1"), &sendCap)
	require.NoError(t, err)
	require.NoError(t, s.Broadcast().UpdateOneID(id).SetStatus(broadcast.StatusSending).Exec(ctx))
	env.PlaceRecipient(ctx, id, fixtures.ContactAliceID)
	env.PlaceRecipient(ctx, id, fixtures.ContactBobID)

	got, err := c.SiteBroadcastsGet(ctx, siteapi.SiteBroadcastsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(id)})
	require.NoError(t, err)
	res, ok := got.(*siteapi.SiteBroadcastResource)
	require.Truef(t, ok, "got %T", got)

	progress, ok := res.Progress.Get()
	require.True(t, ok)
	assert.Equal(t, siteapi.SiteBroadcastLimitSendCap, progress.LimitedBy.Value)
	eta, ok := progress.EstimatedCompletionAt.Get()
	require.True(t, ok)
	assert.WithinDuration(t, time.Now().Add(24*time.Hour), time.Time(eta), time.Minute, "one now, the other a day later")
}
