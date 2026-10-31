package broadcasts_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/patch"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A draft may name a disabled Integration, but Schedule arms the send, so it is refused while the
// Send route cannot send: the Broadcast stays a draft with no time and no job.
func TestScheduleIsRefusedWhileTheBroadcastsIntegrationCannotSend(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := broadcasts.New(env.Bus, testhelper.Renderer(), env.OutboundModule())
	_, err := m.Update(ctx, s, fixtures.BroadcastDraftID, broadcasts.UpdateInput{IntegrationID: patch.Set[int64](fixtures.IntegrationAcmeSesID)})
	require.NoError(t, err, "a draft may name a disabled Integration")

	_, err = m.Schedule(ctx, s, fixtures.BroadcastDraftID, time.Now().Add(time.Hour))

	require.ErrorIs(t, err, outbound.ErrRouteHeld)
	got := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID)
	assert.Equal(t, broadcast.StatusDraft, got.Status)
	assert.Nil(t, got.ScheduledAt)
	assert.Empty(t, env.JobsOf(t, sendKind), "no send job is queued")
}

// A Broadcast whose Integration is disabled after it was scheduled is held by the plan, never sent
// through the default in its place: the plan records the same reason a send would.
func TestPlanHoldsABroadcastWhoseNamedIntegrationCannotSend(t *testing.T) {
	d := newDelivery(t)
	d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastDraftID).SetIntegrationID(fixtures.IntegrationAcmeSesID).ExecX(d.t.Context())

	_, err := d.del.Plan(d.t.Context(), d.s, fixtures.BroadcastDraftID)

	var held *broadcasts.HeldError
	require.ErrorAs(t, err, &held)
	assert.Equal(t, outbound.HoldNoIntegration, held.Reason)
	b := d.env.DB.Broadcast.GetX(d.t.Context(), fixtures.BroadcastDraftID)
	require.NotNil(t, b.HoldReason)
	assert.Equal(t, outbound.HoldNoIntegration, *b.HoldReason)
	assert.Zero(t, d.recipientCount(fixtures.BroadcastDraftID))
	assert.Empty(t, d.box.sent)
}

// The pace and the hold agree: a Broadcast whose Send route cannot send is not paced at once (as
// an unlimited one would be) but held with the reason a send through that route gets.
func TestPaceHoldsABroadcastWhoseRouteCannotSend(t *testing.T) {
	d := newDelivery(t)
	d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastDraftID).SetIntegrationID(fixtures.IntegrationAcmeSesID).ExecX(d.t.Context())

	times, err := d.del.Pace(d.t.Context(), d.s, fixtures.BroadcastDraftID, 3, fixedNow)

	var held *broadcasts.HeldError
	require.ErrorAs(t, err, &held)
	assert.Nil(t, times)
	route, err := d.out.Route(d.t.Context(), d.s, new(int64(fixtures.IntegrationAcmeSesID)))
	require.NoError(t, err)
	assert.Equal(t, route.Hold, held.Reason, "the pace holds for the reason the route gives")
	hold := d.env.DB.Broadcast.GetX(d.t.Context(), fixtures.BroadcastDraftID).HoldReason
	require.NotNil(t, hold)
	assert.Equal(t, outbound.HoldNoIntegration, *hold)
}

// A sending Broadcast whose route cannot send has no estimate: it is blocked, not busy.
func TestProgressHasNoEstimateWhileTheRouteCannotSend(t *testing.T) {
	d := newDelivery(t)
	d.env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeSesID).SetMaxPerSecond(2).ExecX(d.t.Context())
	d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastSendingID).SetIntegrationID(fixtures.IntegrationAcmeSesID).ExecX(d.t.Context())
	d.recipient(fixtures.ContactAliceID)

	detail, err := d.detail(fixedNow)

	require.NoError(t, err)
	require.NotNil(t, detail.Progress)
	assert.Nil(t, detail.Progress.EstimatedCompletion)
}
