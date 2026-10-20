package broadcasts_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/lifecycle"
	"github.com/getsphericon/sphericon/internal/outbound"
)

func (d *delivery) recipientCount(id int64) int {
	n, err := d.env.DB.BroadcastRecipient.Query().Where(broadcastrecipient.BroadcastID(id)).Count(d.ctx)
	require.NoError(d.t, err)
	return n
}

// recordingScheduler is the scheduling seam recording what it is asked, so a test asserts
// the pacing times without waiting for them.
type recordingScheduler struct {
	broadcastID int64
	sends       []broadcasts.Send
}

func (r *recordingScheduler) Schedule(_ context.Context, broadcastID int64, sends []broadcasts.Send) error {
	r.broadcastID = broadcastID
	r.sends = append(r.sends, sends...)
	return nil
}

func (r *recordingScheduler) times() []time.Time {
	out := make([]time.Time, len(r.sends))
	for i, s := range r.sends {
		out[i] = s.At
	}
	return out
}

// run plans, paces and schedules the draft Broadcast and returns what the scheduler saw.
func (d *delivery) run(id int64) (*recordingScheduler, error) {
	sched := &recordingScheduler{}
	err := d.del.Run(d.ctx, d.s, id, sched, time.Now())
	return sched, err
}

// A late or retried plan on a finished Broadcast must not move it back to sending.
func TestPlanIsRefusedFromSentAndFailed(t *testing.T) {
	d := newDelivery(t)

	for id, want := range map[int64]broadcast.Status{
		fixtures.BroadcastSentID:   broadcast.StatusSent,
		fixtures.BroadcastFailedID: broadcast.StatusFailed,
	} {
		_, err := d.del.Plan(d.ctx, d.s, id)
		require.ErrorIs(t, err, lifecycle.ErrInvalidTransition, "broadcast %d", id)
		assert.Equal(t, want, d.env.DB.Broadcast.GetX(d.ctx, id).Status, "broadcast %d", id)
	}
}

func TestPlanCreatesOnePendingRecipientPerEligibleContact(t *testing.T) {
	d := newDelivery(t)

	ids, err := d.del.Plan(d.ctx, d.s, fixtures.BroadcastDraftID)
	require.NoError(t, err)

	require.NotEmpty(t, ids)
	b := d.env.DB.Broadcast.GetX(d.ctx, fixtures.BroadcastDraftID)
	assert.Equal(t, broadcast.StatusSending, b.Status)
	assert.Equal(t, len(ids), b.RecipientsTotal)
	assert.Equal(t, len(ids), d.recipientCount(fixtures.BroadcastDraftID))

	again, err := d.del.Plan(d.ctx, d.s, fixtures.BroadcastDraftID)
	require.NoError(t, err)
	assert.ElementsMatch(t, ids, again, "a re-plan converges on the same rows")
}

// A Broadcast pinned to a From whose domain has no verified sending domain is held at plan
// time, before any recipient row exists, not failed: it keeps its status, records why, and
// plans again once the domain is verified.
func TestPlanHoldsOnUnverifiedFromDomain(t *testing.T) {
	d := newDelivery(t)
	// news.acme.com exists as an *unverified* sending domain (fixture id 2).
	d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastDraftID).SetFromEmail("noreply@news.acme.com").ExecX(d.ctx)

	_, err := d.del.Plan(d.ctx, d.s, fixtures.BroadcastDraftID)
	var held *broadcasts.HeldError
	require.ErrorAs(t, err, &held)
	assert.Equal(t, outbound.HoldUnverifiedDomain, held.Reason)

	got := d.env.DB.Broadcast.GetX(d.ctx, fixtures.BroadcastDraftID)
	assert.Equal(t, broadcast.StatusDraft, got.Status, "a hold is not a failure: the status is unchanged")
	require.NotNil(t, got.HoldReason)
	assert.Equal(t, outbound.HoldUnverifiedDomain, *got.HoldReason)
	assert.Zero(t, d.recipientCount(fixtures.BroadcastDraftID), "no recipient rows while held")

	// Reversible: once the From is on a verified domain the same plan goes through and the
	// hold is cleared.
	d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastDraftID).SetFromEmail("noreply@mail.acme.com").ExecX(d.ctx)
	ids, err := d.del.Plan(d.ctx, d.s, fixtures.BroadcastDraftID)
	require.NoError(t, err)
	assert.NotEmpty(t, ids)
	assert.Nil(t, d.env.DB.Broadcast.GetX(d.ctx, fixtures.BroadcastDraftID).HoldReason)
}

// A template that cannot render affects every recipient, so it is caught at plan time
// (ADR 0015): the Broadcast fails as a whole, before any recipient row exists.
func TestPlanFailsTheWholeBroadcastOnABrokenTemplate(t *testing.T) {
	d := newDelivery(t)
	d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastDraftID).SetSubject("{% if %}broken").ExecX(d.ctx)

	_, err := d.del.Plan(d.ctx, d.s, fixtures.BroadcastDraftID)

	require.Error(t, err)
	require.NotErrorIs(t, err, outbound.ErrInProgress)
	assert.Equal(t, broadcast.StatusFailed, d.env.DB.Broadcast.GetX(d.ctx, fixtures.BroadcastDraftID).Status)
	assert.Zero(t, d.recipientCount(fixtures.BroadcastDraftID), "no recipient rows for a Broadcast that cannot render")
}

func TestPlanFailsOnABrokenSegment(t *testing.T) {
	t.Run("segment of another workspace", func(t *testing.T) {
		d := newDelivery(t)
		d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastDraftID).SetSegmentID(fixtures.SegmentGlobexID).ExecX(d.ctx)

		_, err := d.del.Plan(d.ctx, d.s, fixtures.BroadcastDraftID)

		require.Error(t, err)
		assert.Equal(t, broadcast.StatusFailed, d.env.DB.Broadcast.GetX(d.ctx, fixtures.BroadcastDraftID).Status)
	})

	t.Run("segment with an undecodable definition", func(t *testing.T) {
		d := newDelivery(t)
		d.env.DB.Segment.UpdateOneID(fixtures.SegmentProPlanID).SetDefinition("{not json").ExecX(d.ctx)
		d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastDraftID).SetSegmentID(fixtures.SegmentProPlanID).ExecX(d.ctx)

		_, err := d.del.Plan(d.ctx, d.s, fixtures.BroadcastDraftID)

		require.Error(t, err)
		assert.Equal(t, broadcast.StatusFailed, d.env.DB.Broadcast.GetX(d.ctx, fixtures.BroadcastDraftID).Status)
	})
}

// An empty audience settles the Broadcast at once, so it never hangs in "sending" with no
// recipient to complete it. Fixture broadcast 104 targets a segment that matches nobody.
func TestEmptyAudienceFinalizes(t *testing.T) {
	d := newDelivery(t)

	sched, err := d.run(fixtures.BroadcastEmptyAudienceID)
	require.NoError(t, err)

	assert.Empty(t, sched.sends, "nothing to schedule")
	got := d.env.DB.Broadcast.GetX(d.ctx, fixtures.BroadcastEmptyAudienceID)
	assert.Equal(t, broadcast.StatusSent, got.Status)
	assert.Zero(t, got.RecipientsTotal)
	assert.Zero(t, got.SentCount)
	assert.NotNil(t, got.SentAt)
}

// Finalizing a sent Broadcast again is a no-op: sent_at stays put across concurrent or
// repeated finalizers.
func TestFinalizeIsIdempotent(t *testing.T) {
	d := newDelivery(t)
	id := d.recipient(fixtures.ContactAliceID)
	require.NoError(t, d.del.SendRecipient(d.ctx, d.s, id))
	first := d.broadcastRow()
	require.Equal(t, broadcast.StatusSent, first.Status)
	require.NotNil(t, first.SentAt)

	require.NoError(t, broadcasts.Finalize(d.ctx, d.s, fixtures.BroadcastSendingID))

	assert.True(t, first.SentAt.Equal(*d.broadcastRow().SentAt))
}

// A Broadcast through an Integration limited to 2 per second is paced at one over the rate:
// the sends are spread 500ms apart from now, and the last time is recorded for the estimate.
func TestPaceSpreadsSendsAtTheEffectiveRate(t *testing.T) {
	d := newDelivery(t)
	d.env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetMaxPerSecond(2).ExecX(d.ctx)

	before := time.Now()
	sched, err := d.run(fixtures.BroadcastDraftID)
	require.NoError(t, err)

	times := sched.times()
	require.Greater(t, len(times), 2)
	assert.EqualValues(t, fixtures.BroadcastDraftID, sched.broadcastID)
	assert.WithinDuration(t, before, times[0], 5*time.Second, "starts now")
	for i := 1; i < len(times); i++ {
		assert.Equal(t, 500*time.Millisecond, times[i].Sub(times[i-1]), "one over the rate")
	}
	b := d.env.DB.Broadcast.GetX(d.ctx, fixtures.BroadcastDraftID)
	require.NotNil(t, b.LastScheduledAt)
	assert.WithinDuration(t, times[len(times)-1], *b.LastScheduledAt, time.Millisecond)
	assert.Equal(t, broadcast.StatusSending, b.Status, "pacing adds no status")
}

// A ceiling discovered from the provider paces a Broadcast just like a manual one: the
// effective rate is the lower of the two (ADR 0023).
func TestPaceByTheProviderDiscoveredRate(t *testing.T) {
	d := newDelivery(t)
	d.env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetProviderMaxPerSecond(4).SetMaxPerSecond(100).ExecX(d.ctx)

	sched, err := d.run(fixtures.BroadcastDraftID)
	require.NoError(t, err)

	times := sched.times()
	require.Greater(t, len(times), 2)
	assert.Equal(t, 250*time.Millisecond, times[1].Sub(times[0]), "one over the provider's 4 per second")
	assert.NotNil(t, d.env.DB.Broadcast.GetX(d.ctx, fixtures.BroadcastDraftID).LastScheduledAt)
}

// A Broadcast scheduled for later starts pacing at its scheduled time, not at now.
func TestPaceStartsAtTheScheduledTimeWhenLater(t *testing.T) {
	d := newDelivery(t)
	d.env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetMaxPerDay(86400).ExecX(d.ctx)
	start := time.Now().Add(2 * time.Hour).Truncate(time.Microsecond)
	d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastDraftID).SetScheduledAt(start).ExecX(d.ctx)

	sched, err := d.run(fixtures.BroadcastDraftID)
	require.NoError(t, err)

	times := sched.times()
	require.Greater(t, len(times), 2)
	assert.True(t, start.Equal(times[0]))
	assert.Equal(t, time.Second, times[1].Sub(times[0]), "86400 a day is one a second")
}

// With no limit the Broadcast is planned unpaced: every send at once, no pacing recorded.
func TestPaceLeavesAnUnlimitedBroadcastUnpaced(t *testing.T) {
	d := newDelivery(t)

	sched, err := d.run(fixtures.BroadcastDraftID)
	require.NoError(t, err)

	require.Greater(t, len(sched.sends), 2)
	for _, send := range sched.sends {
		assert.True(t, send.At.IsZero(), "as soon as possible")
	}
	assert.Nil(t, d.env.DB.Broadcast.GetX(d.ctx, fixtures.BroadcastDraftID).LastScheduledAt)
}

// Another Workspace's limit never paces this one: the ceiling lives on the Integration.
func TestAnotherWorkspacesLimitDoesNotPaceThisBroadcast(t *testing.T) {
	d := newDelivery(t)
	d.env.DB.Integration.UpdateOneID(fixtures.IntegrationGlobexID).SetMaxPerSecond(1).ExecX(d.ctx)

	times, err := d.del.Pace(d.ctx, d.s, fixtures.BroadcastDraftID, 50, time.Now())

	require.NoError(t, err)
	assert.Nil(t, times)
}

// The progress estimate and the planner pace by the same interval: with the planned jobs
// behind us, the estimate is the remaining recipients at the planner's step.
func TestProgressEstimateUsesThePlannersInterval(t *testing.T) {
	d := newDelivery(t)
	d.env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetMaxPerSecond(2).ExecX(d.ctx)
	d.recipient(fixtures.ContactAliceID)
	d.recipient(fixtures.ContactBobID)
	now := time.Now()

	p, err := broadcasts.ProgressOf(d.ctx, d.s, d.broadcastRow(), now)

	require.NoError(t, err)
	require.NotNil(t, p.EstimatedCompletion)
	assert.Equal(t, 2*500*time.Millisecond, p.EstimatedCompletion.Sub(now))
}

// A delayed job outlives an unschedule: a Broadcast back in draft is not due, a scheduled
// one is.
func TestDueIsFalseOnlyForADraft(t *testing.T) {
	d := newDelivery(t)

	due, err := broadcasts.Due(d.ctx, d.s, fixtures.BroadcastDraftID, nil)
	require.NoError(t, err)
	assert.False(t, due, "draft: unscheduled after the job was queued")

	due, err = broadcasts.Due(d.ctx, d.s, fixtures.BroadcastScheduledID, nil)
	require.NoError(t, err)
	assert.True(t, due)
}

// A delayed job is bound to the schedule it was enqueued for: after a reschedule (or
// unschedule + schedule) the superseded job must not send the Broadcast early.
func TestDueIsFalseForASupersededScheduledJob(t *testing.T) {
	d := newDelivery(t)
	oldAt := d.env.DB.Broadcast.GetX(d.ctx, fixtures.BroadcastScheduledID).ScheduledAt
	require.NotNil(t, oldAt)
	newAt := oldAt.Add(48 * time.Hour)
	d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastScheduledID).SetScheduledAt(newAt).ExecX(d.ctx)

	due, err := broadcasts.Due(d.ctx, d.s, fixtures.BroadcastScheduledID, oldAt)
	require.NoError(t, err)
	assert.False(t, due, "job for the old schedule is superseded")

	due, err = broadcasts.Due(d.ctx, d.s, fixtures.BroadcastScheduledID, &newAt)
	require.NoError(t, err)
	assert.True(t, due, "job for the current schedule runs")
}

// A delayed job is also superseded by an immediate send (which clears scheduled_at), while
// a retry of the job that legitimately started the send (sending, same scheduled_at) runs.
func TestDueForADelayedJobAcrossSendAndRetry(t *testing.T) {
	d := newDelivery(t)
	at := *d.env.DB.Broadcast.GetX(d.ctx, fixtures.BroadcastScheduledID).ScheduledAt

	d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastScheduledID).SetStatus(broadcast.StatusSending).ExecX(d.ctx)
	due, err := broadcasts.Due(d.ctx, d.s, fixtures.BroadcastScheduledID, &at)
	require.NoError(t, err)
	assert.True(t, due, "retry of the job that started the send")

	d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastScheduledID).ClearScheduledAt().ExecX(d.ctx)
	due, err = broadcasts.Due(d.ctx, d.s, fixtures.BroadcastScheduledID, &at)
	require.NoError(t, err)
	assert.False(t, due, "sent now: the delayed job is superseded")
}
