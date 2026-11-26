package jobs_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/deferralcause"
	"github.com/getsphericon/sphericon/internal/kernel/skipreason"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An Automation's text over the Frequency cap waits at its step until a place frees, for one day at
// most (ADR 0137, ADR 0178). Every Workspace caps SMS at three a day (fixtures/frequency_caps.yml).
// One clock runs the Instance, so the count, the Enrollment's wait and the step's job agree.

// cappedTexter is a test of the cap on ContactTexter's confirmed number: the Instance on the clock
// and its Twilio account. ContactTexter's Enrollment (AutomationRunJobsSMSHiThenTagTexter) stands
// on a text, then a tag.
type cappedTexter struct {
	env     *testhelper.TestEnv
	clock   *testhelper.Clock
	account testhelper.TwilioAccount
	stepper *automations.StepExecution
	start   time.Time
	texted  int
}

func newCappedTexter(t *testing.T) *cappedTexter {
	t.Helper()
	c := testhelper.NewClock(time.Now())
	env := testhelper.Setup(t, testhelper.WithClock(c.Now))
	account := env.AcmeTwilio(t)
	confirmSMS(t, env, fixtures.ContactTexterPhone)
	return &cappedTexter{
		env: env, clock: c, account: account, stepper: stepperAt(env, env.OutboundModule(), c.Now), start: c.Now(),
	}
}

// queueText queues the step ContactTexter's Enrollment stands on, the text, and returns the
// Enrollment.
func (ct *cappedTexter) queueText(t *testing.T) int64 {
	t.Helper()
	runID := int64(fixtures.AutomationRunJobsSMSHiThenTagTexterID)
	queueStep(t, ct.env, runID)
	return runID
}

// broadcastTexts sends n Broadcast texts to the number now, each under a key of its own, and
// requires each Sent: each takes a place.
func (ct *cappedTexter) broadcastTexts(t *testing.T, n int) {
	t.Helper()
	c := ct.env.DB.Contact.GetX(t.Context(), fixtures.ContactTexterID)
	id := int64(ct.account.Integration.ID)
	for range n {
		ct.texted++
		res, err := ct.env.OutboundModule().Send(t.Context(), ct.env.DB.Scoped(fixtures.AcmeID), outboundmessages.Request{
			Ref: outboundmessages.BroadcastRef{BroadcastID: fixtures.BroadcastSmsSentID}, Key: fmt.Sprintf("cap:broadcast:%d", ct.texted),
			Destination: *c.Phone, Contact: c, Content: outboundmessages.SMSContent{Text: "Sale"}, IntegrationID: &id,
		})
		require.NoError(t, err)
		require.Equal(t, outboundmessages.Sent, res.Outcome)
	}
}

// nextStepAt is when the one queued step job is due.
func (ct *cappedTexter) nextStepAt(t *testing.T) time.Time {
	t.Helper()
	at := ct.env.JobSchedules(t, automations.RunStepArgs{}.Kind())
	require.Len(t, at, 1)
	return at[0]
}

// The number got three Broadcast texts an hour apart: the Automation's text an hour after the last
// waits, its step queued for when the first is a day old (with less than a minute of jitter) and
// the wait shown on the Enrollment; then it is sent and the Enrollment goes on.
func TestAnAutomationTextOverTheCapWaitsAtItsStepUntilAPlaceFrees(t *testing.T) {
	ct := newCappedTexter(t)
	ctx := t.Context()
	for range 3 {
		ct.broadcastTexts(t, 1)
		ct.clock.Advance(time.Hour)
	}
	runID := ct.queueText(t)
	waitedFrom := ct.clock.Now()

	require.True(t, ct.env.RunNextStep(t, ct.stepper))

	due := ct.nextStepAt(t)
	release := ct.start.Add(24 * time.Hour)
	assert.False(t, due.Before(release), "queued at the release moment")
	assert.Less(t, due.Sub(release), time.Minute, "with no more than a minute of jitter")
	waiting := ct.env.DB.AutomationRun.GetX(ctx, runID)
	assert.Equal(t, automationrun.StatusActive, waiting.Status)
	assert.Equal(t, ct.env.StepIDAt(t, runID, 0), lo.FromPtr(waiting.CurrentStepID), "the Enrollment waits at its step")
	require.NotNil(t, waiting.DeferredSince)
	assert.WithinDuration(t, waitedFrom, *waiting.DeferredSince, time.Microsecond)
	assert.Equal(t, deferralcause.FrequencyCap, lo.FromPtr(waiting.DeferralCause))
	require.NotNil(t, waiting.ResumeAt)
	assert.WithinDuration(t, due, *waiting.ResumeAt, time.Microsecond, "until when it waits")
	assert.Len(t, ct.env.Twilio.Messages(), 3, "the Automation's text is not sent yet")

	ct.clock.Set(due)
	ct.env.RunQueuedSteps(t, ct.stepper)

	texts := ct.env.Twilio.Messages()
	require.Len(t, texts, 4)
	assert.Contains(t, texts[3].Body, "Hi")
	done := ct.env.DB.AutomationRun.GetX(ctx, runID)
	assert.Equal(t, automationrun.StatusCompleted, done.Status)
	msg := ct.env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID)).OnlyX(ctx)
	assert.Equal(t, outboundmessage.StatusSent, msg.Status)
	assert.Equal(t, deferralcause.FrequencyCap, lo.FromPtr(msg.DeferralCause), "the message keeps why it waited")
}

// Each place that frees is taken by a newer Broadcast text before the Automation's asks again: a
// day after its wait began, the Automation's text is Skipped with frequency_cap and the Enrollment
// moves to its next step.
func TestAnAutomationTextWhosePlaceNeverFreesIsSkippedAtTheBoundAndMovesOn(t *testing.T) {
	ct := newCappedTexter(t)
	ctx := t.Context()
	ct.broadcastTexts(t, 3)
	ct.clock.Advance(time.Hour)
	runID := ct.queueText(t)
	waitedFrom := ct.clock.Now()
	require.True(t, ct.env.RunNextStep(t, ct.stepper))
	ct.clock.Set(ct.nextStepAt(t))
	ct.broadcastTexts(t, 3)

	require.True(t, ct.env.RunNextStep(t, ct.stepper))

	bound := waitedFrom.Add(24 * time.Hour)
	due := ct.nextStepAt(t)
	assert.False(t, due.Before(bound), "the second wait ends at the bound, before the newer texts free a place")
	assert.Less(t, due.Sub(bound), time.Minute)

	ct.clock.Set(due)
	require.True(t, ct.env.RunNextStep(t, ct.stepper))

	moved := ct.env.DB.AutomationRun.GetX(ctx, runID)
	assert.Equal(t, automationrun.StatusActive, moved.Status)
	assert.Equal(t, ct.env.StepIDAt(t, runID, 1), lo.FromPtr(moved.CurrentStepID), "the Enrollment moves to its next step")
	assert.Nil(t, moved.DeferredSince, "and no longer waits")
	skipped := ct.env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID)).OnlyX(ctx)
	assert.Equal(t, outboundmessage.StatusSkipped, skipped.Status)
	assert.Equal(t, skipreason.FrequencyCap, lo.FromPtr(skipped.SkippedReason))
	require.NotNil(t, skipped.DeferredSince)
	assert.WithinDuration(t, waitedFrom, *skipped.DeferredSince, time.Microsecond)
	assert.Len(t, ct.env.Twilio.Messages(), 6, "only the Broadcast texts went")
}
