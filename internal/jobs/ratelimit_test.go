package jobs_test

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/stepping"
	"github.com/getsphericon/sphericon/internal/suspension"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/tracking"
)

// driveRecipients works the recipient jobs the way river does: a snoozed job is not run again before
// its wake time, so the simulated clock jumps to the earliest wake time. It returns the time at which
// the last job was sent.
func driveRecipients(t *testing.T, e *riverEnv, w *jobs.SendRecipientWorker, ids []int64, now time.Time) time.Time {
	t.Helper()
	ctx := e.workCtx()
	wake := map[int64]time.Time{}
	for _, id := range ids {
		wake[id] = now
	}
	for len(wake) > 0 {
		next := now.Add(24 * time.Hour)
		for _, at := range wake {
			if at.Before(next) {
				next = at
			}
		}
		testhelper.Elapse(next.Sub(now))
		now = time.Now()
		for _, id := range ids {
			at, queued := wake[id]
			if !queued || at.After(now) {
				continue
			}
			err := w.Work(ctx, job(jobs.SendRecipientArgs{RecipientID: id, BroadcastID: fixtures.BroadcastDraftID}))
			if err == nil {
				delete(wake, id)
				continue
			}
			var snooze *river.JobSnoozeError
			require.ErrorAs(t, err, &snooze, "a Deferral is a snooze, never a failure")
			assert.GreaterOrEqual(t, snooze.Duration, time.Second, "the wait is floored")
			wake[id] = now.Add(snooze.Duration)
		}
		assert.Nil(t, e.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).HoldReason, "busy is not a hold")
	}
	return now
}

// A Broadcast through an Integration limited to 2 per second: recipient jobs past the
// limit snooze (no attempt consumed, no hold reason), and every recipient is still
// sent exactly once as capacity returns.
func TestBroadcastThroughALimitedIntegrationLosesNoRecipient(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := newRiverEnv(t)
		ctx := e.workCtx()
		now := time.Now()
		mod := outbound.New(e.Bus, e.CoreHolds(), resolvingTo(e.sender), tracking.New("test-secret", "http://local"))
		e.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetMaxPerSecond(2).ExecX(ctx)

		ids, err := planAudience(ctx, e.DB, mod, fixtures.BroadcastDraftID)
		require.NoError(t, err)
		require.Greater(t, len(ids), 2, "the fixture audience must outgrow one second of capacity")
		w := jobs.NewSendRecipientWorker(e.DB, mod)

		start := now
		now = driveRecipients(t, e, w, ids, now)

		// At 2 per second the Broadcast takes about N/2 seconds: the snoozes track the
		// limit instead of running far below it or bursting past it.
		elapsed := now.Sub(start)
		n := float64(len(ids))
		assert.GreaterOrEqual(t, elapsed.Seconds(), n/2-1, "never faster than the limit")
		assert.LessOrEqual(t, elapsed.Seconds(), n/2*1.2+2, "close to the limit, not far below it")
		assert.Len(t, e.sender.sent, len(ids), "every recipient sent exactly once")
		assert.Equal(t, broadcast.StatusSent, e.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).Status)
	})
}

// An Automation step that finds the Integration's limit spent defers: the enrollment
// stays active at the same step, asks to be rescheduled via ResumeAt, and the step
// sends exactly once when capacity returns. Transactional traffic is what spends the
// capacity here, and is itself never slowed.
func TestAutomationStepDefersWhenTransactionalTrafficSpendsTheLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := testhelper.Setup(t)
		ctx := context.Background()
		fs := &fakeSender{}
		mod := outbound.New(env.Bus, env.CoreHolds(), resolvingTo(fs), tracking.New("test-secret", "http://local"))
		env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetMaxPerSecond(1).ExecX(ctx)

		require.NoError(t, stepping.OnEvent(ctx, env.Bus, env.DB.Scoped(fixtures.AcmeID), fixtures.ContactHoldDemoID, automations.Event{Action: "hold_demo", OccurredAt: time.Now()}, 0))
		runID := env.DB.AutomationRun.Query().Where(automationrun.AutomationID(fixtures.AutomationHoldDemoID)).OnlyIDX(ctx)

		for _, key := range []string{"tx:1", "tx:2", "tx:3"} {
			res, err := mod.Send(ctx, env.DB.Scoped(fixtures.AcmeID), outbound.Request{
				Kind: outboundmessage.KindTransactional, Key: key, Destination: "a@example.com",
				Subject: "Receipt", Body: "<p>Hi</p>",
			})
			require.NoError(t, err)
			require.Equal(t, outbound.Sent, res.Outcome, "Transactional is never delayed")
		}
		require.Len(t, fs.sent, 3)

		before := time.Now()
		step := stepperWith(env, mod)
		require.True(t, env.RunNextStep(t, step), "a Deferral is not a failure, so no attempt is consumed")
		require.Equal(t, []map[string]any{stepJob(runID, 0)}, queuedSteps(t, env), "the same step is queued again")
		assert.GreaterOrEqual(t, env.JobSchedules(t, stepping.RunStepArgs{}.Kind())[0].Sub(before), 3*time.Second, "pushed out by the Transactional debt")
		run := env.DB.AutomationRun.GetX(ctx, runID)
		assert.Equal(t, automationrun.StatusActive, run.Status)
		assert.Zero(t, run.CurrentStep, "the enrollment keeps its place")
		assert.Len(t, fs.sent, 3, "the step has not sent")

		testhelper.Elapse(3 * time.Second)
		require.True(t, env.RunNextStep(t, step))
		assert.Len(t, fs.sent, 4, "the step sends once capacity returns")
		assert.Equal(t, 1, env.DB.AutomationRun.GetX(ctx, runID).CurrentStep)
	})
}

// Automation steps and Broadcast recipients draw on one Integration's limit.
func TestAutomationStepAndBroadcastRecipientShareTheLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := newRiverEnv(t)
		ctx := e.workCtx()
		mod := outbound.New(e.Bus, e.CoreHolds(), resolvingTo(e.sender), tracking.New("test-secret", "http://local"))
		e.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetMaxPerSecond(1).ExecX(ctx)

		require.NoError(t, stepping.OnEvent(ctx, e.Bus, e.DB.Scoped(fixtures.AcmeID), fixtures.ContactHoldDemoID, automations.Event{Action: "hold_demo", OccurredAt: time.Now()}, 0))
		runID := e.DB.AutomationRun.Query().Where(automationrun.AutomationID(fixtures.AutomationHoldDemoID)).OnlyIDX(ctx)
		require.True(t, e.RunNextStep(t, stepperWith(e.TestEnv, mod)))
		require.Equal(t, []map[string]any{stepJob(runID, 1)}, queuedSteps(t, e.TestEnv), "the first send takes the only token and the Enrollment moves on")
		require.Len(t, e.sender.sent, 1)

		ids, err := planAudience(ctx, e.DB, mod, fixtures.BroadcastDraftID)
		require.NoError(t, err)
		w := jobs.NewSendRecipientWorker(e.DB, mod)
		err = w.Work(ctx, job(jobs.SendRecipientArgs{RecipientID: ids[0], BroadcastID: fixtures.BroadcastDraftID}))
		var snooze *river.JobSnoozeError
		require.ErrorAs(t, err, &snooze, "the Broadcast recipient finds the Automation's spend")
	})
}

// A Broadcast larger than the Workspace's Send cap (ADR 0067) stays sending, no hold: the
// recipient past the bucket's tokens defers, asked again within minutes, and goes out as the
// 24-hour window frees up; every recipient is sent exactly once.
func TestABroadcastLargerThanTheSendCapStaysSendingAndFinishes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := newRiverEnv(t)
		ctx := e.workCtx()
		mod := outbound.New(e.Bus, e.CoreHolds(), resolvingTo(e.sender), tracking.New("test-secret", "http://local"), outbound.WithSendCap())
		ids, err := planAudience(ctx, e.DB, mod, fixtures.BroadcastDraftID)
		require.NoError(t, err)
		// One recipient more than the cap, so the test waits for one token, not a day per recipient.
		sendCap := len(ids) - 1
		_, err = suspension.SetSendCap(ctx, e.Bus, fixtures.AcmeID, suspension.StaffMember("op-1"), &sendCap)
		require.NoError(t, err)
		w := jobs.NewSendRecipientWorker(e.DB, mod)

		start := time.Now()
		end := driveRecipients(t, e, w, ids, start)

		assert.GreaterOrEqual(t, end.Sub(start), 24*time.Hour/time.Duration(sendCap), "the last one waits for a token")
		assert.Len(t, e.sender.sent, len(ids), "every recipient sent exactly once")
		assert.Equal(t, broadcast.StatusSent, e.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).Status)
	})
}
