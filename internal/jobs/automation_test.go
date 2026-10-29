package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/messaging"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/tag"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/stepping"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/tracking"
)

// passingClock is a clock that has passed a year more every time it is read: whatever time a
// wait recorded, the queue's run of its follow-up finds that time gone by. A test that drives an
// Automation to its end lets waits pass this way.
func passingClock() func() time.Time {
	now := time.Now()
	return func() time.Time {
		now = now.Add(365 * 24 * time.Hour)
		return now
	}
}

// stepperOf is the stepping module over the sender resolver, its clock passing every wait.
func stepperOf(env *testhelper.TestEnv, resolver outbound.Senders) *stepping.Module {
	return stepperWith(env, newMod(env, resolver), stepping.WithClock(passingClock()))
}

// stepperWith is the stepping module over mod, as configured.
func stepperWith(env *testhelper.TestEnv, mod *outbound.Module, opts ...stepping.Option) *stepping.Module {
	return stepping.New(env.Bus, mod, noFence, opts...)
}

// queueStep queues the step job of an Enrollment standing on a fixture, as the enroll or the
// previous step would have.
func queueStep(t *testing.T, env *testhelper.TestEnv, runID int64) {
	t.Helper()
	env.EnqueueJob(t, stepping.RunStepArgs{RunID: runID, Step: env.DB.AutomationRun.GetX(t.Context(), runID).CurrentStep})
}

// stepOnce executes the step an Enrollment of Acme stands on, and returns what the step returned.
func stepOnce(t *testing.T, env *testhelper.TestEnv, mod *stepping.Module, runID int64) error {
	t.Helper()
	return mod.Step(t.Context(), env.DB.Scoped(fixtures.AcmeID), runID, env.DB.AutomationRun.GetX(t.Context(), runID).CurrentStep)
}

// queuedSteps are the args of the step jobs waiting in the queue.
func queuedSteps(t *testing.T, env *testhelper.TestEnv) []map[string]any {
	t.Helper()
	return env.JobsOf(t, stepping.RunStepArgs{}.Kind())
}

// stepJob is the args of a queued step job as JobsOf reads them.
func stepJob(runID int64, step int) map[string]any {
	return map[string]any{"run_id": float64(runID), "step": float64(step)}
}

// triggerOf is the event that enrolls a fixture Automation.
func triggerOf(t *testing.T, env *testhelper.TestEnv, automationID int64) string {
	t.Helper()
	return env.DB.Automation.GetX(context.Background(), automationID).TriggerEvent
}

// enrollVia enrolls the Contact through the Automation's own trigger and returns its one run.
func enrollVia(t *testing.T, env *testhelper.TestEnv, automationID, contactID int64) int64 {
	t.Helper()
	require.NoError(t, jobs.EvaluateTrigger(context.Background(), env.Bus, env.DB.Scoped(fixtures.AcmeID), contactID, triggerOf(t, env, automationID), 0))
	runIDs := enrollmentsOf(t, env, automationID, contactID)
	require.Len(t, runIDs, 1)
	return runIDs[0]
}

// enrollmentsOf is the ids of the Contact's Enrollments in the Automation.
func enrollmentsOf(t *testing.T, env *testhelper.TestEnv, automationID, contactID int64) []int64 {
	t.Helper()
	return env.DB.AutomationRun.Query().
		Where(automationrun.AutomationID(automationID), automationrun.ContactID(contactID)).
		IDsX(context.Background())
}

func TestAutomationEnrollAndRun(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	alice := env.DB.Contact.GetX(ctx, fixtures.ContactAliceID)
	runID := enrollVia(t, env, fixtures.AutomationJobsGreetingID, alice.ID)

	// Enroll-once: a second matching event creates no new run.
	require.NoError(t, jobs.EvaluateTrigger(ctx, env.Bus, env.DB.Scoped(fixtures.AcmeID), alice.ID, triggerOf(t, env, fixtures.AutomationJobsGreetingID), 0))
	assert.Equal(t, []int64{runID}, enrollmentsOf(t, env, fixtures.AutomationJobsGreetingID, alice.ID))

	fs := &fakeSender{}
	env.RunQueuedSteps(t, stepperOf(env, resolvingTo(fs)))

	// Two email steps fired (the wait step does not send).
	require.Len(t, fs.sent, 2)
	assert.Equal(t, "Hi "+*alice.FirstName, fs.sent[0].Subject)
	assert.NotContains(t, fs.sent[0].HTML, "{{")

	assert.Equal(t, automationrun.StatusCompleted, env.DB.AutomationRun.GetX(ctx, runID).Status)

	// Each automation send publishes email.sent onto the transactional outbox, so an
	// automation send is segmentable through the same Event log as broadcast sends.
	// Two email steps mean two send facts for this contact.
	sent := env.OutboxCount(t, "email.sent", map[string]any{"email": *alice.Email})
	assert.Equal(t, 2, sent)
}

// An opt-out from this automation's source (or a suppression) exits the
// enrollment instead of sending: a run never silently skips its emails.
// Leo opted out of the Welcome series' own sending source.
func TestAutomationExitsOnUnsubscribe(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	runID := enrollVia(t, env, fixtures.AutomationWelcomeSeriesID, fixtures.ContactOptedOutOfWelcomeID)

	fs := &fakeSender{}
	env.RunQueuedSteps(t, stepperOf(env, resolvingTo(fs)))

	assert.Empty(t, fs.sent, "ineligible destination receives no email")
	assert.Equal(t, automationrun.StatusExited, env.DB.AutomationRun.GetX(ctx, runID).Status)
	assert.Empty(t, queuedSteps(t, env), "an exit queues no further step")
}

// A suppression exits the enrollment too (the global hard floor).
func TestAutomationExitsOnSuppression(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	runID := enrollVia(t, env, fixtures.AutomationJobsSingleEmailID, fixtures.ContactComplainedID)

	fs := &fakeSender{}
	env.RunQueuedSteps(t, stepperOf(env, resolvingTo(fs)))

	assert.Empty(t, fs.sent)
	assert.Equal(t, automationrun.StatusExited, env.DB.AutomationRun.GetX(ctx, runID).Status)
}

// A per-source opt-out is scoped: unsubscribing from "broadcasts" does not block
// an automation (a different sending source). Bob is opted out of broadcasts only.
func TestAutomationUnaffectedByBroadcastUnsubscribe(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	runID := enrollVia(t, env, fixtures.AutomationJobsSingleEmailID, fixtures.ContactBobID)

	fs := &fakeSender{}
	env.RunQueuedSteps(t, stepperOf(env, resolvingTo(fs)))

	assert.Len(t, fs.sent, 1, "broadcasts opt-out must not block the automation")
	assert.Equal(t, automationrun.StatusCompleted, env.DB.AutomationRun.GetX(ctx, runID).Status)
}

// An automation send carries an unsubscribe footer scoped to the automation, so
// the recipient can opt out (which exits the enrollment: see the endpoint test).
func TestAutomationSendIncludesUnsubscribeFooter(t *testing.T) {
	env := testhelper.Setup(t)
	enrollVia(t, env, fixtures.AutomationJobsSingleEmailID, fixtures.ContactLiamID)

	fs := &fakeSender{}
	mod := outbound.New(env.Bus, env.Holds, resolvingTo(fs), tracking.New("secret", "https://app.test"))
	env.RunQueuedSteps(t, stepperWith(env, mod))

	require.Len(t, fs.sent, 1)
	assert.Contains(t, fs.sent[0].HTML, "https://app.test/e/u/", "automation email carries an unsubscribe link")
	// RFC 8058 one-click header (ADR 0012), scoped to this automation source.
	assert.Contains(t, fs.sent[0].ListUnsubscribeURL, "https://app.test/e/u/",
		"automation sets a one-click unsubscribe URL")
}

func TestAutomationInactiveDoesNotEnroll(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	require.NoError(t, jobs.EvaluateTrigger(ctx, env.Bus, env.DB.Scoped(fixtures.AcmeID), fixtures.ContactAliceID, triggerOf(t, env, fixtures.AutomationWinBackID), 0))
	assert.Empty(t, enrollmentsOf(t, env, fixtures.AutomationWinBackID, fixtures.ContactAliceID), "the win-back automation is a draft")
}

func TestAutomationWaitParksTheEnrollmentAndQueuesTheNextStepForTheResumeTime(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	runID := enrollVia(t, env, fixtures.AutomationJobsWaitFirstID, fixtures.ContactAliceID)

	fs := &fakeSender{}
	require.True(t, env.RunNextStep(t, stepperWith(env, newMod(env, resolvingTo(fs)))))

	run := env.DB.AutomationRun.GetX(ctx, runID)
	require.NotNil(t, run.ResumeAt)
	assert.Equal(t, 1, run.CurrentStep)
	assert.Equal(t, []map[string]any{stepJob(runID, 1)}, queuedSteps(t, env), "the step after the wait is queued, and nothing else")
	assert.WithinDuration(t, *run.ResumeAt, env.JobSchedules(t, stepping.RunStepArgs{}.Kind())[0], time.Second, "for the resume time")
	assert.Empty(t, fs.sent)
}

// A step that runs before the Enrollment's recorded resume time (a retried wait) queues itself
// for that time and does nothing else: the email after the wait is not sent early.
func TestAStepRunningBeforeTheResumeTimeQueuesItselfAndDoesNothingElse(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	runID := enrollVia(t, env, fixtures.AutomationJobsWaitFirstID, fixtures.ContactAliceID)
	fs := &fakeSender{}
	mod := stepperWith(env, newMod(env, resolvingTo(fs)))

	require.True(t, env.RunNextStep(t, mod)) // the wait
	require.True(t, env.RunNextStep(t, mod)) // the email, delivered before the resume time

	assert.Empty(t, fs.sent, "the email after the wait is not sent before the resume time")
	run := env.DB.AutomationRun.GetX(ctx, runID)
	assert.Equal(t, 1, run.CurrentStep, "the Enrollment stays on the step after the wait")
	assert.Equal(t, automationrun.StatusActive, run.Status)
	assert.Equal(t, []map[string]any{stepJob(runID, 1)}, queuedSteps(t, env), "the step waits for its time")
	assert.WithinDuration(t, *run.ResumeAt, env.JobSchedules(t, stepping.RunStepArgs{}.Kind())[0], time.Second)
}

// A step job delivered again after its transaction committed (the queue completes a job in a
// separate step) finds the Enrollment on the next step: it does nothing and queues nothing.
func TestAStepJobExecutedTwiceMovesTheEnrollmentOnce(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	runID := enrollVia(t, env, fixtures.AutomationJobsWaitFirstID, fixtures.ContactAliceID)
	mod := stepperOf(env, resolvingTo(&fakeSender{}))
	acme := env.DB.Scoped(fixtures.AcmeID)

	require.NoError(t, mod.Step(ctx, acme, runID, 0))
	afterFirst := queuedSteps(t, env)
	runAfterFirst := env.DB.AutomationRun.GetX(ctx, runID)

	require.NoError(t, mod.Step(ctx, acme, runID, 0))

	assert.Equal(t, afterFirst, queuedSteps(t, env), "the second execution queues nothing")
	runAfterSecond := env.DB.AutomationRun.GetX(ctx, runID)
	assert.Equal(t, 1, runAfterSecond.CurrentStep, "the Enrollment moved once")
	assert.Equal(t, runAfterFirst.UpdatedAt, runAfterSecond.UpdatedAt, "the second execution wrote nothing")
	assert.Equal(t, runAfterFirst.ResumeAt, runAfterSecond.ResumeAt)
}

// A tag step delivered twice tags once and moves once.
func TestATagStepJobExecutedTwiceQueuesTheNextStepOnce(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	runID := enrollVia(t, env, fixtures.AutomationJobsSystemTagID, fixtures.ContactAliceID)
	mod := stepperOf(env, resolvingTo(&fakeSender{}))
	acme := env.DB.Scoped(fixtures.AcmeID)

	require.NoError(t, mod.Step(ctx, acme, runID, 0))
	require.NoError(t, mod.Step(ctx, acme, runID, 0))

	assert.Equal(t, []map[string]any{stepJob(runID, 0), stepJob(runID, 1)}, queuedSteps(t, env), "the enrolling job and one follow-up")
	assert.Equal(t, 1, env.DB.AutomationRun.GetX(ctx, runID).CurrentStep)
}

// A Contact erased after its step was queued takes the Enrollment with it: the step finds
// nothing, ends quietly and sends nothing.
func TestAStepOfAnEnrollmentErasedWithItsContactEndsQuietly(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	runID := enrollVia(t, env, fixtures.AutomationJobsSingleEmailID, fixtures.ContactLiamID)
	env.DB.Contact.DeleteOneID(fixtures.ContactLiamID).ExecX(ctx)
	fs := &fakeSender{}

	env.RunQueuedSteps(t, stepperOf(env, resolvingTo(fs)))

	assert.False(t, env.DB.AutomationRun.Query().Where(automationrun.ID(runID)).ExistX(ctx), "the Enrollment went with its Contact")
	assert.Empty(t, fs.sent)
	assert.Empty(t, queuedSteps(t, env))
}

// A hold on the source (here: a suspended Workspace) leaves the enrollment exactly
// where it was and queues the same step for later; the run is neither failed nor exited.
func TestAutomationHeldStepWaitsAndResumes(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	runID := enrollVia(t, env, fixtures.AutomationHoldDemoID, fixtures.ContactHoldDemoID)

	fs := &fakeSender{}
	mod := stepperOf(env, resolvingTo(fs))
	env.SuspendWorkspace(ctx, fixtures.AcmeID, time.Now())

	require.True(t, env.RunNextStep(t, mod))

	run := env.DB.AutomationRun.GetX(ctx, runID)
	assert.Equal(t, automationrun.StatusActive, run.Status)
	assert.Zero(t, run.CurrentStep, "a hold does not advance the enrollment")
	assert.Equal(t, []map[string]any{stepJob(runID, 0)}, queuedSteps(t, env), "the worker reschedules the same step")
	assert.Empty(t, fs.sent)

	env.UnsuspendWorkspace(ctx, fixtures.AcmeID)
	env.RunQueuedSteps(t, mod)
	assert.Len(t, fs.sent, 1, "after the unfreeze the step sends")
}

// Every email an Automation sends goes through the Integration it names, not the default.
func TestAnAutomationSendsThroughItsOwnIntegration(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	env.DB.Automation.UpdateOneID(fixtures.AutomationJobsGreetingID).SetIntegrationID(fixtures.IntegrationAcmeMailgunID).ExecX(ctx)
	enrollVia(t, env, fixtures.AutomationJobsGreetingID, fixtures.ContactAliceID)

	var through []int64
	fs := &fakeSender{}
	resolver := &SendersMock{EmailSenderFunc: func(_ context.Context, _ *ent.Scoped, integ *ent.Integration) (messaging.EmailSender, error) {
		through = append(through, integ.ID)
		return fs, nil
	}}
	env.RunQueuedSteps(t, stepperOf(env, resolver))

	require.Len(t, fs.sent, 2)
	assert.Equal(t, []int64{fixtures.IntegrationAcmeMailgunID, fixtures.IntegrationAcmeMailgunID}, through)
	sent := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationID(fixtures.AutomationJobsGreetingID)).AllX(ctx)
	require.Len(t, sent, 2)
	for _, msg := range sent {
		require.NotNil(t, msg.IntegrationID)
		assert.EqualValues(t, fixtures.IntegrationAcmeMailgunID, *msg.IntegrationID)
	}
}

// Naming no Integration is the Workspace default, the one every Automation used before.
func TestAnAutomationWithoutAnIntegrationSendsThroughTheDefault(t *testing.T) {
	env := testhelper.Setup(t)
	enrollVia(t, env, fixtures.AutomationJobsGreetingID, fixtures.ContactAliceID)

	var through []int64
	resolver := &SendersMock{EmailSenderFunc: func(_ context.Context, _ *ent.Scoped, integ *ent.Integration) (messaging.EmailSender, error) {
		through = append(through, integ.ID)
		return &fakeSender{}, nil
	}}
	env.RunQueuedSteps(t, stepperOf(env, resolver))

	assert.Equal(t, []int64{fixtures.IntegrationAcmeDefaultID, fixtures.IntegrationAcmeDefaultID}, through)
}

// A named Integration that cannot send, because it is disabled or archived, holds the step,
// which asks again later: nothing falls back to the default, and the Enrollment does not move.
func TestAnAutomationStepIsHeldWhenItsIntegrationCannotSend(t *testing.T) {
	unusable := map[string]func(env *testhelper.TestEnv){
		"disabled": func(env *testhelper.TestEnv) {
			env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeMailgunID).SetEnabled(false).ExecX(context.Background())
		},
		"archived": func(env *testhelper.TestEnv) {
			require.NoError(t, env.DB.Scoped(fixtures.AcmeID).Integration().Discard(context.Background(), fixtures.IntegrationAcmeMailgunID))
		},
	}
	for name, makeUnusable := range unusable {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			ctx := context.Background()
			env.DB.Automation.UpdateOneID(fixtures.AutomationJobsGreetingID).SetIntegrationID(fixtures.IntegrationAcmeMailgunID).ExecX(ctx)
			makeUnusable(env)
			runID := enrollVia(t, env, fixtures.AutomationJobsGreetingID, fixtures.ContactAliceID)

			fs := &fakeSender{}
			require.True(t, env.RunNextStep(t, stepperOf(env, resolvingTo(fs))))

			run := env.DB.AutomationRun.GetX(ctx, runID)
			assert.Equal(t, automationrun.StatusActive, run.Status)
			assert.Zero(t, run.CurrentStep, "a hold does not advance the enrollment")
			assert.Equal(t, []map[string]any{stepJob(runID, 0)}, queuedSteps(t, env), "the worker reschedules the same step")
			assert.Empty(t, fs.sent)
		})
	}
}

// An Enrollment exited while a step is in flight stays exited: the late step's own
// outcome (here a send that can never be composed) finishes quietly instead of
// overwriting the exit.
func TestAutomationExitedMidStepStaysExitedAndTheLateStepFinishesQuietly(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	run := env.DB.AutomationRun.GetX(ctx, fixtures.AutomationRunJobsBrokenTemplateID)
	queueStep(t, env, run.ID)

	// The Contact unsubscribes while the step is already running: the provider lookup
	// is inside the step, after the run was read.
	resolver := &SendersMock{EmailSenderFunc: func(ctx context.Context, sc *ent.Scoped, _ *ent.Integration) (messaging.EmailSender, error) {
		_, err := automations.Exit(ctx, sc, run.AutomationID, run.ContactID)
		require.NoError(t, err)
		return &fakeSender{}, nil
	}}

	env.RunQueuedSteps(t, stepperOf(env, resolver))

	assert.Equal(t, automationrun.StatusExited, env.DB.AutomationRun.GetX(ctx, run.ID).Status)
	assert.Empty(t, queuedSteps(t, env))
}

func TestStepOnAnExitedRunSendsNothingAndStaysPut(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	fs := &fakeSender{}
	runID := int64(fixtures.AutomationRunJobsExitedOnEmailID)
	queueStep(t, env, runID)

	env.RunQueuedSteps(t, stepperOf(env, resolvingTo(fs)))

	assert.Empty(t, fs.sent, "nothing is sent after an exit")
	run := env.DB.AutomationRun.GetX(ctx, runID)
	assert.Equal(t, automationrun.StatusExited, run.Status)
	assert.Equal(t, 0, run.CurrentStep)
	assert.Empty(t, queuedSteps(t, env))
}

func TestTagStepOnAnExitedRunChangesNoTagAndDoesNotAdvance(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	runID := int64(fixtures.AutomationRunJobsExitedOnTagID)
	run := env.DB.AutomationRun.GetX(ctx, runID)
	queueStep(t, env, runID)

	env.RunQueuedSteps(t, stepperOf(env, resolvingTo(&fakeSender{})))

	assert.False(t, env.DB.Contact.GetX(ctx, run.ContactID).QueryTags().Where(tag.Name("system-made")).ExistX(ctx), "the exited Contact was not tagged")
	assert.Equal(t, 0, env.DB.AutomationRun.GetX(ctx, runID).CurrentStep)
	assert.Empty(t, queuedSteps(t, env))
}

func TestWaitStepOnAnExitedRunDoesNotSetResumeAt(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	runID := int64(fixtures.AutomationRunJobsExitedOnWaitID) // exited with the wait step next
	queueStep(t, env, runID)

	env.RunQueuedSteps(t, stepperOf(env, resolvingTo(&fakeSender{})))

	run := env.DB.AutomationRun.GetX(ctx, runID)
	assert.Equal(t, automationrun.StatusExited, run.Status)
	assert.Nil(t, run.ResumeAt)
	assert.Equal(t, 1, run.CurrentStep)
	assert.Empty(t, queuedSteps(t, env))
}

// An Enrollment finishes on the steps it began with, however the Automation is edited after
// (ADR 0049): the next Enrollment begins on the new steps.
func TestAutomationEnrollmentFinishesOnItsOwnVersionAfterAnEdit(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	mod := automations.New(env.Cipher, env.Bus, testhelper.Renderer())

	early := enrollVia(t, env, fixtures.AutomationJobsEditableID, fixtures.ContactAliceID)
	// The first step (the wait) runs; the Enrollment now stands on the email step.
	fs := &fakeSender{}
	stepper := stepperOf(env, resolvingTo(fs))
	require.True(t, env.RunNextStep(t, stepper))

	// The owner edits the steps: a wait is inserted ahead and the email is reworded. Were the
	// Enrollment to read the live steps, its index 1 would now be the new wait.
	_, err := mod.Update(ctx, acme, fixtures.AutomationJobsEditableID, automations.UpdateInput{Steps: &[]automations.Step{
		{Type: automations.StepWait, Seconds: 1},
		{Type: automations.StepWait, Seconds: 2},
		{Type: automations.StepEmail, Subject: "New steps", Body: "<p>new</p>"},
	}})
	require.NoError(t, err)

	late := enrollVia(t, env, fixtures.AutomationJobsEditableID, fixtures.ContactCarolID)

	env.RunQueuedSteps(t, stepper)

	require.Len(t, fs.sent, 2)
	assert.Equal(t, "Old steps", fs.sent[0].Subject, "the Enrollment begun before the edit finished on its own version")
	assert.Equal(t, "New steps", fs.sent[1].Subject, "the Enrollment begun after the edit used the new steps")
	assert.Equal(t, automationrun.StatusCompleted, env.DB.AutomationRun.GetX(ctx, early).Status)
	assert.NotEqual(t, env.DB.AutomationRun.GetX(ctx, early).VersionID, env.DB.AutomationRun.GetX(ctx, late).VersionID)
}

// An Event from an Automation's own email never enrolls the Contact into that Automation, so an
// `email.opened` Automation cannot be fed by the mail it sends; another Automation on the same
// trigger still enrolls (ADR 0053).
func TestAnAutomationIsNotStartedByItsOwnEmail(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)

	require.NoError(t, jobs.EvaluateTrigger(ctx, env.Bus, acme, fixtures.ContactAliceID, events.NameEmailOpened, fixtures.AutomationOnOpenID))
	assert.Empty(t, enrollmentsOf(t, env, fixtures.AutomationOnOpenID, fixtures.ContactAliceID), "the open of its own email enrolls nobody")

	require.NoError(t, jobs.EvaluateTrigger(ctx, env.Bus, acme, fixtures.ContactAliceID, events.NameEmailOpened, fixtures.AutomationWelcomeSeriesID))
	assert.Len(t, enrollmentsOf(t, env, fixtures.AutomationOnOpenID, fixtures.ContactAliceID), 1, "the open of another Automation's email starts On open")
}
