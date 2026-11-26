package jobs_test

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/riverqueue/river"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/sending/linkclick"
	"github.com/getsphericon/sphericon/internal/domain/sending/messagetoken"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/deferralcause"
	"github.com/getsphericon/sphericon/internal/testhelper"
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
		mod := outboundmessages.New(e.Bus, e.CoreHolds(), resolvingTo(e.sender), messagetoken.New("test-secret", "http://local", time.Now), e.OutboundModules(e.Bus, time.Now), outboundmessages.Deps{}, outboundmessages.Config{}, time.Now)
		e.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetMaxPerSecond(2).ExecX(ctx)

		ids, err := planAudience(ctx, e.DB, mod, fixtures.BroadcastDraftID)
		require.NoError(t, err)
		require.Greater(t, len(ids), 2, "the fixture audience must outgrow one second of capacity")
		w := jobs.NewSendRecipientWorker(e.DB, deliveryOf(mod))

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

// A Broadcast larger than its Workspace's Send cap is not refused and not held: the planner paces it
// by the Provider limit alone (none here, so all at once), the recipients the Send cap lets out go
// at once, the rest snooze within the Send cap's bound, the progress names the Send cap with an
// estimate a token away, and the Broadcast stays sending until the window frees up and it finishes.
func TestBroadcastLargerThanTheSendCapStaysSendingAndFinishesAsTheWindowFreesUp(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := newRiverEnv(t)
		ctx := e.workCtx()
		now := time.Now()
		mod := outboundmessages.New(e.Bus, e.CoreHolds(), resolvingTo(e.sender), messagetoken.New("test-secret", "http://local", time.Now), e.OutboundModules(e.Bus, time.Now), outboundmessages.Deps{}, outboundmessages.Config{SendCaps: true}, time.Now)

		ids, err := planAudience(ctx, e.DB, mod, fixtures.BroadcastDraftID)
		require.NoError(t, err)
		require.Greater(t, len(ids), 2, "the fixture audience must outgrow a Send cap of more than one")
		sendCap := len(ids) - 1
		e.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSendCap(sendCap).ExecX(ctx)
		perToken := 24 * time.Hour / time.Duration(sendCap)

		detail, err := broadcasts.New(e.Bus, testhelper.Renderer(), mod, linkclick.New()).
			Detail(ctx, e.DB.Scoped(fixtures.AcmeID), e.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID))
		require.NoError(t, err)
		require.NotNil(t, detail.Progress)
		assert.Equal(t, deferralcause.SendCap, detail.Progress.LimitedBy)
		require.NotNil(t, detail.Progress.EstimatedCompletion)
		assert.WithinDuration(t, now.Add(perToken), *detail.Progress.EstimatedCompletion, time.Second, "the one beyond the bucket waits a token")

		w := jobs.NewSendRecipientWorker(e.DB, deliveryOf(mod))
		start := now
		now = driveRecipients(t, e, w, ids, now)

		elapsed := now.Sub(start)
		assert.GreaterOrEqual(t, elapsed, perToken-time.Second, "never faster than the Send cap")
		assert.LessOrEqual(t, elapsed, perToken+10*time.Minute, "the last one goes within the bound of its token")
		finished := e.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID)
		assert.Len(t, e.sender.sent, len(ids), "every recipient sent exactly once")
		assert.Equal(t, broadcast.StatusSent, finished.Status)
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
		mod := outboundmessages.New(env.Bus, env.CoreHolds(), resolvingTo(fs), messagetoken.New("test-secret", "http://local", time.Now), env.OutboundModules(env.Bus, time.Now), outboundmessages.Deps{}, outboundmessages.Config{}, time.Now)
		env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetMaxPerSecond(1).ExecX(ctx)

		require.NoError(t, testhelper.Module[*automations.StepExecution](env).OnEvent(ctx, env.DB.Scoped(fixtures.AcmeID), automations.OnEventInput{ContactID: fixtures.ContactHoldDemoID, Event: automations.Event{Action: "hold_demo", OccurredAt: time.Now()}}))
		runID := env.DB.AutomationRun.Query().Where(automationrun.AutomationID(fixtures.AutomationHoldDemoID)).OnlyIDX(ctx)

		for _, key := range []string{"tx:1", "tx:2", "tx:3"} {
			res, err := mod.Send(ctx, env.DB.Scoped(fixtures.AcmeID), outboundmessages.Request{
				Ref: outboundmessages.TransactionalRef{}, Key: key, Destination: "a@example.com",
				Content: outboundmessages.EmailContent{Subject: "Receipt", Body: "<p>Hi</p>"},
			})
			require.NoError(t, err)
			require.Equal(t, outboundmessages.Sent, res.Outcome, "Transactional is never delayed")
		}
		require.Len(t, fs.sent, 3)

		before := time.Now()
		step := stepperWith(env, mod)
		require.True(t, env.RunNextStep(t, step), "a Deferral is not a failure, so no attempt is consumed")
		require.Equal(t, []map[string]any{stepJob(t, env, runID, 0)}, queuedSteps(t, env), "the same step is queued again")
		assert.GreaterOrEqual(t, env.JobSchedules(t, automations.RunStepArgs{}.Kind())[0].Sub(before), 3*time.Second, "pushed out by the Transactional debt")
		run := env.DB.AutomationRun.GetX(ctx, runID)
		assert.Equal(t, automationrun.StatusActive, run.Status)
		assert.Equal(t, env.StepIDAt(t, runID, 0), lo.FromPtr(run.CurrentStepID), "the enrollment keeps its place")
		assert.Len(t, fs.sent, 3, "the step has not sent")

		testhelper.Elapse(3 * time.Second)
		require.True(t, env.RunNextStep(t, step))
		assert.Len(t, fs.sent, 4, "the step sends once capacity returns")
		assert.Equal(t, automationrun.StatusCompleted, env.DB.AutomationRun.GetX(ctx, runID).Status, "and leaves its one Step")
	})
}

// An Automation step that finds the Workspace's Send cap spent reschedules through its Enrollment
// like any Deferral, but asks again within the Send cap's bound (about 10 minutes) rather than
// after the day the bucket needs, so a Send cap a Staff member raises is noticed within minutes.
func TestAutomationStepDeferredBySendCapAsksAgainWithinTheBound(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := testhelper.Setup(t)
		ctx := context.Background()
		fs := &fakeSender{}
		mod := outboundmessages.New(env.Bus, env.CoreHolds(), resolvingTo(fs), messagetoken.New("test-secret", "http://local", time.Now), env.OutboundModules(env.Bus, time.Now), outboundmessages.Deps{}, outboundmessages.Config{SendCaps: true}, time.Now)
		env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSendCap(1).ExecX(ctx)
		// The day's one token went to another send just now.
		env.DB.SendCapLimiter.UpdateOneID(fixtures.SendCapLimiterAcmeID).SetDayTokens(0).SetRefilledAt(time.Now()).ExecX(ctx)

		require.NoError(t, testhelper.Module[*automations.StepExecution](env).OnEvent(ctx, env.DB.Scoped(fixtures.AcmeID), automations.OnEventInput{ContactID: fixtures.ContactHoldDemoID, Event: automations.Event{Action: "hold_demo", OccurredAt: time.Now()}}))
		runID := env.DB.AutomationRun.Query().Where(automationrun.AutomationID(fixtures.AutomationHoldDemoID)).OnlyIDX(ctx)

		before := time.Now()
		ran := env.RunNextStep(t, stepperWith(env, mod))
		require.True(t, ran, "a Deferral is not a failure")
		queued := queuedSteps(t, env)
		require.Equal(t, []map[string]any{stepJob(t, env, runID, 0)}, queued, "the same step is queued again")
		wait := env.JobSchedules(t, automations.RunStepArgs{}.Kind())[0].Sub(before)
		assert.Positive(t, wait)
		assert.LessOrEqual(t, wait, 10*time.Minute, "asks again within the bound, not after a day")
		run := env.DB.AutomationRun.GetX(ctx, runID)
		assert.Equal(t, automationrun.StatusActive, run.Status)
		assert.Equal(t, env.StepIDAt(t, runID, 0), lo.FromPtr(run.CurrentStepID), "the enrollment keeps its place")
		assert.Empty(t, fs.sent, "the step has not sent")
	})
}

// An Automation send the Send cap defers records its wait on the Enrollment (ADR 0178): since when
// it waits at the step, the cause and until when. A second Deferral at the step keeps the first
// moment and moves the until. Once the send goes, the Enrollment's record is cleared and the
// Outbound message keeps since when and why it waited.
func TestAnAutomationSendTheSendCapDefersRecordsItsWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := testhelper.Setup(t)
		ctx := context.Background()
		fs := &fakeSender{}
		mod := outboundmessages.New(env.Bus, env.CoreHolds(), resolvingTo(fs), messagetoken.New("test-secret", "http://local", time.Now), env.OutboundModules(env.Bus, time.Now), outboundmessages.Deps{}, outboundmessages.Config{SendCaps: true}, time.Now)
		env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSendCap(1).ExecX(ctx)
		// The day's one token went to another send just now.
		env.DB.SendCapLimiter.UpdateOneID(fixtures.SendCapLimiterAcmeID).SetDayTokens(0).SetRefilledAt(time.Now()).ExecX(ctx)
		require.NoError(t, testhelper.Module[*automations.StepExecution](env).OnEvent(ctx, env.DB.Scoped(fixtures.AcmeID), automations.OnEventInput{ContactID: fixtures.ContactHoldDemoID, Event: automations.Event{Action: "hold_demo", OccurredAt: time.Now()}}))
		runID := env.DB.AutomationRun.Query().Where(automationrun.AutomationID(fixtures.AutomationHoldDemoID)).OnlyIDX(ctx)
		step := stepperWith(env, mod)

		first := time.Now()
		require.True(t, env.RunNextStep(t, step))
		deferred := env.DB.AutomationRun.GetX(ctx, runID)
		firstRetry := env.JobSchedules(t, automations.RunStepArgs{}.Kind())[0]
		require.NotNil(t, deferred.DeferredSince)
		assert.WithinDuration(t, first, *deferred.DeferredSince, time.Microsecond, "since the first Deferral at the step")
		assert.Equal(t, deferralcause.SendCap, lo.FromPtr(deferred.DeferralCause))
		require.NotNil(t, deferred.ResumeAt)
		assert.WithinDuration(t, firstRetry, *deferred.ResumeAt, time.Microsecond, "until it is tried again")

		testhelper.Elapse(time.Until(firstRetry))
		require.True(t, env.RunNextStep(t, step))
		again := env.DB.AutomationRun.GetX(ctx, runID)
		require.NotNil(t, again.DeferredSince)
		assert.WithinDuration(t, first, *again.DeferredSince, time.Microsecond, "a second Deferral keeps the first moment")
		assert.Equal(t, deferralcause.SendCap, lo.FromPtr(again.DeferralCause))
		require.NotNil(t, again.ResumeAt)
		assert.True(t, again.ResumeAt.After(firstRetry), "and waits until its own retry")
		assert.Empty(t, fs.sent, "the step has not sent")

		// A token is back.
		env.DB.SendCapLimiter.UpdateOneID(fixtures.SendCapLimiterAcmeID).SetDayTokens(1).SetRefilledAt(time.Now()).ExecX(ctx)
		testhelper.Elapse(time.Until(*again.ResumeAt))
		require.True(t, env.RunNextStep(t, step))
		require.Len(t, fs.sent, 1, "the step sends")
		sent := env.DB.AutomationRun.GetX(ctx, runID)
		assert.Equal(t, automationrun.StatusCompleted, sent.Status)
		assert.Nil(t, sent.DeferredSince, "the Enrollment no longer waits")
		assert.Nil(t, sent.DeferralCause)
		msg := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID)).OnlyX(ctx)
		assert.Equal(t, outboundmessage.StatusSent, msg.Status)
		require.NotNil(t, msg.DeferredSince)
		assert.WithinDuration(t, first, *msg.DeferredSince, time.Microsecond, "the message keeps since when it waited")
		assert.Equal(t, deferralcause.SendCap, lo.FromPtr(msg.DeferralCause), "and why")
	})
}

// A send that never waited leaves no wait on its Outbound message.
func TestAnAutomationSendThatNeverWaitedRecordsNoWait(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	fs := &fakeSender{}
	mod := outboundmessages.New(env.Bus, env.CoreHolds(), resolvingTo(fs), messagetoken.New("test-secret", "http://local", time.Now), env.OutboundModules(env.Bus, time.Now), outboundmessages.Deps{}, outboundmessages.Config{}, time.Now)
	require.NoError(t, testhelper.Module[*automations.StepExecution](env).OnEvent(ctx, env.DB.Scoped(fixtures.AcmeID), automations.OnEventInput{ContactID: fixtures.ContactHoldDemoID, Event: automations.Event{Action: "hold_demo", OccurredAt: time.Now()}}))
	runID := env.DB.AutomationRun.Query().Where(automationrun.AutomationID(fixtures.AutomationHoldDemoID)).OnlyIDX(ctx)

	require.True(t, env.RunNextStep(t, stepperWith(env, mod)))

	msg := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID)).OnlyX(ctx)
	assert.Equal(t, outboundmessage.StatusSent, msg.Status)
	assert.Nil(t, msg.DeferredSince)
	assert.Nil(t, msg.DeferralCause)
}

// Automation steps and Broadcast recipients draw on one Integration's limit.
func TestAutomationStepAndBroadcastRecipientShareTheLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := newRiverEnv(t)
		ctx := e.workCtx()
		mod := outboundmessages.New(e.Bus, e.CoreHolds(), resolvingTo(e.sender), messagetoken.New("test-secret", "http://local", time.Now), e.OutboundModules(e.Bus, time.Now), outboundmessages.Deps{}, outboundmessages.Config{}, time.Now)
		e.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetMaxPerSecond(1).ExecX(ctx)

		require.NoError(t, testhelper.Module[*automations.StepExecution](e.TestEnv).OnEvent(ctx, e.DB.Scoped(fixtures.AcmeID), automations.OnEventInput{ContactID: fixtures.ContactHoldDemoID, Event: automations.Event{Action: "hold_demo", OccurredAt: time.Now()}}))
		runID := e.DB.AutomationRun.Query().Where(automationrun.AutomationID(fixtures.AutomationHoldDemoID)).OnlyIDX(ctx)
		require.True(t, e.RunNextStep(t, stepperWith(e.TestEnv, mod)))
		require.Equal(t, automationrun.StatusCompleted, e.DB.AutomationRun.GetX(ctx, runID).Status, "the first send takes the only token and the Enrollment moves on")
		require.Len(t, e.sender.sent, 1)

		ids, err := planAudience(ctx, e.DB, mod, fixtures.BroadcastDraftID)
		require.NoError(t, err)
		w := jobs.NewSendRecipientWorker(e.DB, deliveryOf(mod))
		err = w.Work(ctx, job(jobs.SendRecipientArgs{RecipientID: ids[0], BroadcastID: fixtures.BroadcastDraftID}))
		var snooze *river.JobSnoozeError
		require.ErrorAs(t, err, &snooze, "the Broadcast recipient finds the Automation's spend")
	})
}
