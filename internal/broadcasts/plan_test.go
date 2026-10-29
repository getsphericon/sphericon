package broadcasts_test

import (
	"context"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/holds"
	"github.com/getsphericon/sphericon/internal/lifecycle"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/outbound"
)

func (d *delivery) recipientCount(id int64) int {
	n, err := d.env.DB.BroadcastRecipient.Query().Where(broadcastrecipient.BroadcastID(id)).Count(d.t.Context())
	require.NoError(d.t, err)
	return n
}

// fixedNow is the clock of every pacing test: the module is handed the time, so no test
// reads or waits for the real one.
var fixedNow = time.Date(2030, time.March, 4, 10, 0, 0, 0, time.UTC)

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
	err := d.del.Run(d.t.Context(), d.s, id, sched, fixedNow)
	return sched, err
}

// A late or retried plan on a finished Broadcast must not move it back to sending.
func TestPlanIsRefusedFromSentAndFailed(t *testing.T) {
	d := newDelivery(t)

	for id, want := range map[int64]broadcast.Status{
		fixtures.BroadcastSentID:   broadcast.StatusSent,
		fixtures.BroadcastFailedID: broadcast.StatusFailed,
	} {
		_, err := d.del.Plan(d.t.Context(), d.s, id)
		require.ErrorIs(t, err, lifecycle.ErrInvalidTransition, "broadcast %d", id)
		status := d.env.DB.Broadcast.GetX(d.t.Context(), id).Status
		assert.Equal(t, want, status, "broadcast %d", id)
	}
}

func TestPlanCreatesOnePendingRecipientPerEligibleContact(t *testing.T) {
	d := newDelivery(t)

	ids, err := d.del.Plan(d.t.Context(), d.s, fixtures.BroadcastDraftID)
	require.NoError(t, err)

	require.NotEmpty(t, ids)
	b := d.env.DB.Broadcast.GetX(d.t.Context(), fixtures.BroadcastDraftID)
	assert.Equal(t, broadcast.StatusSending, b.Status)
	assert.Equal(t, len(ids), d.recipientCount(fixtures.BroadcastDraftID))

	again, err := d.del.Plan(d.t.Context(), d.s, fixtures.BroadcastDraftID)
	require.NoError(t, err)
	assert.ElementsMatch(t, ids, again, "a re-plan converges on the same rows")
}

// A Broadcast pinned to a From whose domain has no verified sending domain is held at plan
// time, before any recipient row exists, not failed: it keeps its status, records why, and
// plans again once the domain is verified.
func TestPlanHoldsOnUnverifiedFromDomain(t *testing.T) {
	d := newDelivery(t)
	// news.acme.com exists as an *unverified* sending domain (fixture id 2).
	d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastDraftID).SetFromEmail("noreply@news.acme.com").ExecX(d.t.Context())

	_, err := d.del.Plan(d.t.Context(), d.s, fixtures.BroadcastDraftID)
	var held *broadcasts.HeldError
	require.ErrorAs(t, err, &held)
	assert.Equal(t, outbound.HoldUnverifiedDomain, held.Reason)

	got := d.env.DB.Broadcast.GetX(d.t.Context(), fixtures.BroadcastDraftID)
	assert.Equal(t, broadcast.StatusDraft, got.Status, "a hold is not a failure: the status is unchanged")
	require.NotNil(t, got.HoldReason)
	assert.Equal(t, outbound.HoldUnverifiedDomain, *got.HoldReason)
	assert.Zero(t, d.recipientCount(fixtures.BroadcastDraftID), "no recipient rows while held")

	// Reversible: once the From is on a verified domain the same plan goes through and the
	// hold is cleared.
	d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastDraftID).SetFromEmail("noreply@mail.acme.com").ExecX(d.t.Context())
	ids, err := d.del.Plan(d.t.Context(), d.s, fixtures.BroadcastDraftID)
	require.NoError(t, err)
	assert.NotEmpty(t, ids)
	hold := d.env.DB.Broadcast.GetX(d.t.Context(), fixtures.BroadcastDraftID).HoldReason
	assert.Nil(t, hold)
}

// A template that cannot render affects every recipient, so it is caught at plan time
// (ADR 0015): the Broadcast fails as a whole, before any recipient row exists.
func TestPlanFailsTheWholeBroadcastOnABrokenTemplate(t *testing.T) {
	d := newDelivery(t)
	d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastDraftID).SetSubject("{% if %}broken").ExecX(d.t.Context())

	_, err := d.del.Plan(d.t.Context(), d.s, fixtures.BroadcastDraftID)

	require.Error(t, err)
	require.NotErrorIs(t, err, outbound.ErrInProgress)
	status := d.env.DB.Broadcast.GetX(d.t.Context(), fixtures.BroadcastDraftID).Status
	assert.Equal(t, broadcast.StatusFailed, status)
	assert.Zero(t, d.recipientCount(fixtures.BroadcastDraftID), "no recipient rows for a Broadcast that cannot render")
}

func TestPlanFailsOnABrokenSegment(t *testing.T) {
	t.Run("segment of another workspace", func(t *testing.T) {
		d := newDelivery(t)
		d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastDraftID).SetSegmentID(fixtures.SegmentGlobexID).ExecX(d.t.Context())

		_, err := d.del.Plan(d.t.Context(), d.s, fixtures.BroadcastDraftID)

		require.Error(t, err)
		status := d.env.DB.Broadcast.GetX(d.t.Context(), fixtures.BroadcastDraftID).Status
		assert.Equal(t, broadcast.StatusFailed, status)
	})

	t.Run("segment with an undecodable definition", func(t *testing.T) {
		d := newDelivery(t)
		d.env.DB.Segment.UpdateOneID(fixtures.SegmentProPlanID).SetCondition("{not json").ExecX(d.t.Context())
		d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastDraftID).SetSegmentID(fixtures.SegmentProPlanID).ExecX(d.t.Context())

		_, err := d.del.Plan(d.t.Context(), d.s, fixtures.BroadcastDraftID)

		require.Error(t, err)
		status := d.env.DB.Broadcast.GetX(d.t.Context(), fixtures.BroadcastDraftID).Status
		assert.Equal(t, broadcast.StatusFailed, status)
	})
}

// An empty audience settles the Broadcast at once, so it never hangs in "sending" with no
// recipient to complete it. Fixture broadcast 104 targets a segment that matches nobody.
func TestEmptyAudienceFinalizes(t *testing.T) {
	d := newDelivery(t)

	sched, err := d.run(fixtures.BroadcastEmptyAudienceID)
	require.NoError(t, err)

	assert.Empty(t, sched.sends, "nothing to schedule")
	got := d.env.DB.Broadcast.GetX(d.t.Context(), fixtures.BroadcastEmptyAudienceID)
	assert.Equal(t, broadcast.StatusSent, got.Status)
	figures, err := broadcasts.StatsOf(d.t.Context(), d.s, fixtures.BroadcastEmptyAudienceID)
	require.NoError(t, err)
	assert.Zero(t, figures[fixtures.BroadcastEmptyAudienceID])
	assert.NotNil(t, got.SentAt)
}

// Finalizing a sent Broadcast again is a no-op: sent_at stays put across concurrent or
// repeated finalizers.
func TestFinalizeIsIdempotent(t *testing.T) {
	d := newDelivery(t)
	id := d.recipient(fixtures.ContactAliceID)
	require.NoError(t, d.del.SendRecipient(d.t.Context(), d.s, id))
	first := d.broadcastRow()
	require.Equal(t, broadcast.StatusSent, first.Status)
	require.NotNil(t, first.SentAt)

	require.NoError(t, broadcasts.Finalize(d.t.Context(), d.s, fixtures.BroadcastSendingID))

	assert.True(t, first.SentAt.Equal(*d.broadcastRow().SentAt))
}

// A Broadcast through an Integration limited to 2 per second is paced at one over the rate:
// the sends are spread 500ms apart from now, and the last time is recorded for the estimate.
func TestPaceSpreadsSendsAtTheEffectiveRate(t *testing.T) {
	d := newDelivery(t)
	d.env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetMaxPerSecond(2).ExecX(d.t.Context())

	sched, err := d.run(fixtures.BroadcastDraftID)
	require.NoError(t, err)

	times := sched.times()
	require.Greater(t, len(times), 2)
	assert.EqualValues(t, fixtures.BroadcastDraftID, sched.broadcastID)
	assert.True(t, fixedNow.Equal(times[0]), "starts now")
	for i := 1; i < len(times); i++ {
		assert.Equal(t, 500*time.Millisecond, times[i].Sub(times[i-1]), "one over the rate")
	}
	b := d.env.DB.Broadcast.GetX(d.t.Context(), fixtures.BroadcastDraftID)
	require.NotNil(t, b.LastScheduledAt)
	assert.WithinDuration(t, times[len(times)-1], *b.LastScheduledAt, time.Millisecond)
	assert.Equal(t, broadcast.StatusSending, b.Status, "pacing adds no status")
}

// A Broadcast that names an Integration is paced by that Integration's limit, not the default's.
func TestPaceUsesTheBroadcastsOwnIntegration(t *testing.T) {
	d := newDelivery(t)
	d.env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeMailgunID).SetMaxPerSecond(2).ExecX(d.t.Context())
	d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastDraftID).SetIntegrationID(fixtures.IntegrationAcmeMailgunID).ExecX(d.t.Context())

	sched, err := d.run(fixtures.BroadcastDraftID)
	require.NoError(t, err)

	times := sched.times()
	require.Greater(t, len(times), 2, "paced by the Broadcast's own Integration")
	gap := times[1].Sub(times[0])
	assert.Equal(t, 500*time.Millisecond, gap)
}

// The plan's Preflight asks about the Broadcast's own Integration: one that is disabled holds
// the plan rather than falling back to the default.
func TestPlanIsHeldWhenTheBroadcastsIntegrationIsDisabled(t *testing.T) {
	d := newDelivery(t)
	d.env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeMailgunID).SetEnabled(false).ExecX(d.t.Context())
	d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastDraftID).SetIntegrationID(fixtures.IntegrationAcmeMailgunID).ExecX(d.t.Context())

	_, err := d.run(fixtures.BroadcastDraftID)

	var held *broadcasts.HeldError
	require.ErrorAs(t, err, &held)
	assert.Equal(t, "no_integration", held.Reason)
}

// A ceiling discovered from the provider paces a Broadcast just like a manual one: the
// effective rate is the lower of the two (ADR 0023).
func TestPaceByTheProviderDiscoveredRate(t *testing.T) {
	d := newDelivery(t)
	d.env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetProviderMaxPerSecond(4).SetMaxPerSecond(100).ExecX(d.t.Context())

	sched, err := d.run(fixtures.BroadcastDraftID)
	require.NoError(t, err)

	times := sched.times()
	require.Greater(t, len(times), 2)
	assert.Equal(t, 250*time.Millisecond, times[1].Sub(times[0]), "one over the provider's 4 per second")
	last := d.env.DB.Broadcast.GetX(d.t.Context(), fixtures.BroadcastDraftID).LastScheduledAt
	assert.NotNil(t, last)
}

// A Broadcast scheduled for later starts pacing at its scheduled time, not at now.
func TestPaceStartsAtTheScheduledTimeWhenLater(t *testing.T) {
	d := newDelivery(t)
	d.env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetMaxPerDay(86400).ExecX(d.t.Context())
	start := fixedNow.Add(2 * time.Hour)
	d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastDraftID).SetScheduledAt(start).ExecX(d.t.Context())

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
	last := d.env.DB.Broadcast.GetX(d.t.Context(), fixtures.BroadcastDraftID).LastScheduledAt
	assert.Nil(t, last)
}

// Another Workspace's limit never paces this one: the ceiling lives on the Integration.
func TestAnotherWorkspacesLimitDoesNotPaceThisBroadcast(t *testing.T) {
	d := newDelivery(t)
	d.env.DB.Integration.UpdateOneID(fixtures.IntegrationGlobexID).SetMaxPerSecond(1).ExecX(d.t.Context())

	times, err := d.del.Pace(d.t.Context(), d.s, fixtures.BroadcastDraftID, 50, fixedNow)

	require.NoError(t, err)
	assert.Nil(t, times)
}

// The progress estimate and the planner pace by the same interval: with the planned jobs
// behind us, the estimate is the remaining recipients at the planner's step.
func TestProgressEstimateUsesThePlannersInterval(t *testing.T) {
	d := newDelivery(t)
	d.env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetMaxPerSecond(2).ExecX(d.t.Context())
	d.recipient(fixtures.ContactAliceID)
	d.recipient(fixtures.ContactBobID)
	now := fixedNow

	detail, err := broadcasts.DetailOf(d.t.Context(), d.s, d.broadcastRow(), now)

	require.NoError(t, err)
	p := detail.Progress
	require.NotNil(t, p.EstimatedCompletion)
	assert.Equal(t, 2*500*time.Millisecond, p.EstimatedCompletion.Sub(now))
}

// The estimate follows the Broadcast's own Integration too, not the default's.
func TestProgressEstimateUsesTheBroadcastsOwnIntegration(t *testing.T) {
	d := newDelivery(t)
	d.env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeMailgunID).SetMaxPerSecond(2).ExecX(d.t.Context())
	d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastSendingID).SetIntegrationID(fixtures.IntegrationAcmeMailgunID).ExecX(d.t.Context())
	d.recipient(fixtures.ContactAliceID)
	d.recipient(fixtures.ContactBobID)

	detail, err := broadcasts.DetailOf(d.t.Context(), d.s, d.broadcastRow(), fixedNow)

	require.NoError(t, err)
	require.NotNil(t, detail.Progress.EstimatedCompletion)
	eta := detail.Progress.EstimatedCompletion.Sub(fixedNow)
	assert.Equal(t, 2*500*time.Millisecond, eta)
}

// A delayed job outlives an unschedule: a Broadcast back in draft is not due, a scheduled
// one is.
func TestDueIsFalseOnlyForADraft(t *testing.T) {
	d := newDelivery(t)

	due, err := broadcasts.Due(d.t.Context(), d.s, fixtures.BroadcastDraftID, nil)
	require.NoError(t, err)
	assert.False(t, due, "draft: unscheduled after the job was queued")

	due, err = broadcasts.Due(d.t.Context(), d.s, fixtures.BroadcastScheduledID, nil)
	require.NoError(t, err)
	assert.True(t, due)
}

// A delayed job is bound to the schedule it was enqueued for: after a reschedule (or
// unschedule + schedule) the superseded job must not send the Broadcast early.
func TestDueIsFalseForASupersededScheduledJob(t *testing.T) {
	d := newDelivery(t)
	oldAt := d.env.DB.Broadcast.GetX(d.t.Context(), fixtures.BroadcastScheduledID).ScheduledAt
	require.NotNil(t, oldAt)
	newAt := oldAt.Add(48 * time.Hour)
	d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastScheduledID).SetScheduledAt(newAt).ExecX(d.t.Context())

	due, err := broadcasts.Due(d.t.Context(), d.s, fixtures.BroadcastScheduledID, oldAt)
	require.NoError(t, err)
	assert.False(t, due, "job for the old schedule is superseded")

	due, err = broadcasts.Due(d.t.Context(), d.s, fixtures.BroadcastScheduledID, &newAt)
	require.NoError(t, err)
	assert.True(t, due, "job for the current schedule runs")
}

// A delayed job is also superseded by an immediate send (which clears scheduled_at), while
// a retry of the job that legitimately started the send (sending, same scheduled_at) runs.
func TestDueForADelayedJobAcrossSendAndRetry(t *testing.T) {
	d := newDelivery(t)
	at := *d.env.DB.Broadcast.GetX(d.t.Context(), fixtures.BroadcastScheduledID).ScheduledAt

	d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastScheduledID).SetStatus(broadcast.StatusSending).ExecX(d.t.Context())
	due, err := broadcasts.Due(d.t.Context(), d.s, fixtures.BroadcastScheduledID, &at)
	require.NoError(t, err)
	assert.True(t, due, "retry of the job that started the send")

	d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastScheduledID).ClearScheduledAt().ExecX(d.t.Context())
	due, err = broadcasts.Due(d.t.Context(), d.s, fixtures.BroadcastScheduledID, &at)
	require.NoError(t, err)
	assert.False(t, due, "sent now: the delayed job is superseded")
}

// A Workspace freeze holds the plan: the Broadcast keeps its status, records why and no
// recipient row exists; it resumes once the freeze lifts (story 6).
func TestPlanHoldsOnAWorkspaceFreezeAndResumes(t *testing.T) {
	d := newDelivery(t)
	d.env.SuspendWorkspace(d.t.Context(), fixtures.AcmeID, fixedNow)

	_, err := d.del.Plan(d.t.Context(), d.s, fixtures.BroadcastDraftID)

	var held *broadcasts.HeldError
	require.ErrorAs(t, err, &held)
	assert.Equal(t, holds.ReasonSuspended, held.Reason)
	b := d.env.DB.Broadcast.GetX(d.t.Context(), fixtures.BroadcastDraftID)
	assert.Equal(t, broadcast.StatusDraft, b.Status)
	require.NotNil(t, b.HoldReason)
	assert.Equal(t, holds.ReasonSuspended, *b.HoldReason)
	assert.Zero(t, d.recipientCount(fixtures.BroadcastDraftID))

	d.env.UnsuspendWorkspace(d.t.Context(), fixtures.AcmeID)
	ids, err := d.del.Plan(d.t.Context(), d.s, fixtures.BroadcastDraftID)
	require.NoError(t, err)
	assert.NotEmpty(t, ids)
	hold := d.env.DB.Broadcast.GetX(d.t.Context(), fixtures.BroadcastDraftID).HoldReason
	assert.Nil(t, hold, "the hold clears once the plan goes through")
}

// A Workspace with no usable Integration holds the plan the same way.
func TestPlanHoldsWithNoIntegration(t *testing.T) {
	d := newDelivery(t)
	d.res.err = messaging.ErrNoProvider

	_, err := d.del.Plan(d.t.Context(), d.s, fixtures.BroadcastDraftID)

	var held *broadcasts.HeldError
	require.ErrorAs(t, err, &held)
	assert.Equal(t, outbound.HoldNoIntegration, held.Reason)
	b := d.env.DB.Broadcast.GetX(d.t.Context(), fixtures.BroadcastDraftID)
	assert.Equal(t, broadcast.StatusDraft, b.Status)
	require.NotNil(t, b.HoldReason)
	assert.Zero(t, d.recipientCount(fixtures.BroadcastDraftID))
}

// A plan that fails the Broadcast is recorded once and finishes cleanly: Run returns nil, so
// no queue retries it, and the Broadcast is failed with no recipient row.
func TestRunFinishesCleanlyWhenThePlanFailsTheBroadcast(t *testing.T) {
	d := newDelivery(t)
	d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastDraftID).SetSubject("{% if %}broken").ExecX(d.t.Context())

	sched, err := d.run(fixtures.BroadcastDraftID)

	require.NoError(t, err)
	assert.Empty(t, sched.sends)
	status := d.env.DB.Broadcast.GetX(d.t.Context(), fixtures.BroadcastDraftID).Status
	assert.Equal(t, broadcast.StatusFailed, status)
	assert.Zero(t, d.recipientCount(fixtures.BroadcastDraftID))

	again, err := d.run(fixtures.BroadcastDraftID)
	require.ErrorIs(t, err, lifecycle.ErrInvalidTransition, "a re-run of the failed Broadcast is a stale job")
	assert.Empty(t, again.sends)
}

// A plan that was refused leaves a held Broadcast's hold alone: a stale job records nothing.
func TestStalePlanLeavesTheHoldReasonAlone(t *testing.T) {
	d := newDelivery(t)
	d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastSentID).SetHoldReason("kept").ExecX(d.t.Context())
	d.env.SuspendWorkspace(d.t.Context(), fixtures.AcmeID, fixedNow)

	_, err := d.del.Plan(d.t.Context(), d.s, fixtures.BroadcastSentID)

	require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
	hold := d.env.DB.Broadcast.GetX(d.t.Context(), fixtures.BroadcastSentID).HoldReason
	require.NotNil(t, hold)
	assert.Equal(t, "kept", *hold)
}

// Pacing only decides when a send runs: an unsubscribe or Suppression that lands after
// planning is honoured when the send runs, and the recipient is skipped, never mailed.
func TestSuppressionDuringPacingIsHonouredAtSendTime(t *testing.T) {
	d := newDelivery(t)
	d.env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetMaxPerSecond(1).ExecX(d.t.Context())
	_, err := d.run(fixtures.BroadcastDraftID)
	require.NoError(t, err)

	alice := d.env.DB.Contact.GetX(d.t.Context(), fixtures.ContactAliceID)
	d.env.SuppressDestination(d.t.Context(), *alice.Email)
	rec := d.env.DB.BroadcastRecipient.Query().
		Where(broadcastrecipient.BroadcastID(fixtures.BroadcastDraftID), broadcastrecipient.ContactID(alice.ID)).OnlyX(d.t.Context())

	require.NoError(t, d.del.SendRecipient(d.t.Context(), d.s, rec.ID))

	assert.Equal(t, outboundmessage.StatusSkipped, d.messageOf(rec.ID).Status)
	for _, m := range d.box.sent {
		assert.NotEqual(t, *alice.Email, m.To)
	}
}

// Lowering the limit after a Broadcast was planned takes effect through Deferral: the sends
// already planned are deferred instead of exceeding the new ceiling. The token bucket reads the
// clock, so the test runs under testing/synctest, whose time stands still between the sends
// (on the core holds, see newCoreDelivery).
func TestLoweringTheLimitMidSendDefersTheRemainingRecipients(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		d := newCoreDelivery(t)
		sched, err := d.run(fixtures.BroadcastDraftID) // planned unlimited: every send due now
		require.NoError(t, err)
		require.Greater(t, len(sched.sends), 2)
		d.env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetMaxPerSecond(1).ExecX(d.t.Context())

		results := make([]error, len(sched.sends))
		for i, send := range sched.sends {
			results[i] = d.del.SendRecipient(d.t.Context(), d.s, send.RecipientID)
		}

		require.NoError(t, results[0], "the first takes the second's one token")
		for _, err := range results[1:] {
			var de *broadcasts.DeferredError
			require.ErrorAs(t, err, &de)
		}
		assert.Len(t, d.box.sent, 1, "one second's worth went out")
		hold := d.env.DB.Broadcast.GetX(d.t.Context(), fixtures.BroadcastDraftID).HoldReason
		assert.Nil(t, hold, "busy is not a hold")
	})
}

// A deferred recipient waits behind only the recipients that are awake and ahead of it: one
// asleep on its own Deferral is not queueing for a token, so it must not stretch the wait
// of later ones (ADR 0023, "scaled by the backlog ahead"). Under testing/synctest, as above.
func TestDeferralBacklogIgnoresRecipientsAsleepOnTheirOwnDeferral(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		d := newCoreDelivery(t)
		sched, err := d.run(fixtures.BroadcastDraftID)
		require.NoError(t, err)
		require.Greater(t, len(sched.sends), 2)
		d.env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetMaxPerSecond(1).ExecX(d.t.Context())
		ids := make([]int64, len(sched.sends))
		for i, send := range sched.sends {
			ids[i] = send.RecipientID
		}
		slices.Sort(ids)

		require.NoError(t, d.del.SendRecipient(d.t.Context(), d.s, ids[0])) // spends the second's one token
		var first *broadcasts.DeferredError
		require.ErrorAs(t, d.del.SendRecipient(d.t.Context(), d.s, ids[1]), &first) // now asleep

		var second *broadcasts.DeferredError
		require.ErrorAs(t, d.del.SendRecipient(d.t.Context(), d.s, ids[2]), &second)
		// Recipient 1 is pending with a lower id but asleep: nobody is ahead, so the wait is
		// about a second plus jitter, not the two tokens' worth it would be.
		assert.LessOrEqual(t, second.Delay, 1300*time.Millisecond)
	})
}

// Whatever the rate, the start and the number of sends, pacing starts at the start, puts the
// sends exactly one step (one over the rate) apart and records the last one on the Broadcast.
func TestPaceSpreadsEverySendOneStepApartFromTheStart(t *testing.T) {
	d := newDelivery(t)
	rapid.Check(t, func(rt *rapid.T) {
		n := rapid.IntRange(1, 500).Draw(rt, "n")
		perSecond := rapid.IntRange(1, 1000).Draw(rt, "perSecond")
		later := time.Duration(rapid.Int64Range(0, int64(24*time.Hour/time.Microsecond)).Draw(rt, "later")) * time.Microsecond
		start := fixedNow.Add(later)
		d.env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetMaxPerSecond(perSecond).ExecX(d.t.Context())
		d.env.DB.Broadcast.UpdateOneID(fixtures.BroadcastDraftID).SetScheduledAt(start).ExecX(d.t.Context())
		step := time.Second / time.Duration(perSecond)

		times, err := d.del.Pace(d.t.Context(), d.s, fixtures.BroadcastDraftID, n, fixedNow)

		require.NoError(rt, err)
		require.Len(rt, times, n)
		assert.Equal(rt, lo.Times(n, func(i int) time.Duration { return time.Duration(i) * step }), sinceStart(times, start))
		last := d.env.DB.Broadcast.GetX(d.t.Context(), fixtures.BroadcastDraftID).LastScheduledAt
		require.NotNil(rt, last)
		assert.WithinDuration(rt, times[n-1], *last, time.Microsecond)
	})
}

// sinceStart is how long after start each time is.
func sinceStart(times []time.Time, start time.Time) []time.Duration {
	return lo.Map(times, func(at time.Time, _ int) time.Duration { return at.Sub(start) })
}

// A queued job runs only a Broadcast that was claimed for it (scheduled or sending). One
// that an unschedule returned to draft after the job's Due check must stay a draft: the
// plan of a direct caller may claim a draft, the plan of a queued job may not.
func TestQueuedRunRefusesADraftUnscheduledAfterItsDueCheck(t *testing.T) {
	d := newDelivery(t)

	err := d.del.RunClaimed(d.t.Context(), d.s, fixtures.BroadcastDraftID, &recordingScheduler{}, fixedNow)

	require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
	status := d.env.DB.Broadcast.GetX(d.t.Context(), fixtures.BroadcastDraftID).Status
	assert.Equal(t, broadcast.StatusDraft, status)
	assert.Zero(t, d.recipientCount(fixtures.BroadcastDraftID))
}

func TestQueuedRunPlansAScheduledBroadcast(t *testing.T) {
	d := newDelivery(t)

	sched := &recordingScheduler{}
	err := d.del.RunClaimed(d.t.Context(), d.s, fixtures.BroadcastScheduledID, sched, fixedNow)

	require.NoError(t, err)
	status := d.env.DB.Broadcast.GetX(d.t.Context(), fixtures.BroadcastScheduledID).Status
	assert.Equal(t, broadcast.StatusSending, status)
	assert.NotEmpty(t, sched.sends)
}
