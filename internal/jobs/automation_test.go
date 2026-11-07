package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automationstep"
	"github.com/getsphericon/sphericon/internal/infra/messaging"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/tag"
	"github.com/getsphericon/sphericon/internal/domain/sending/messagetoken"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/stepexecution"
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

// stepperOf is the Step execution use case over the sender resolver, its clock passing every wait.
func stepperOf(env *testhelper.TestEnv, resolver outboundmessages.Senders) *stepexecution.UseCase {
	return stepperWith(env, newMod(env, resolver), stepexecution.WithClock(passingClock()))
}

// stepperWith is the Step execution use case over mod, as configured.
func stepperWith(env *testhelper.TestEnv, mod *outboundmessages.Module, opts ...stepexecution.Option) *stepexecution.UseCase {
	return stepexecution.New(env.Bus, mod, noFence, opts...)
}

// queueStep queues the step job of an Enrollment standing on a fixture, as the enroll or the
// previous step would have.
func queueStep(t *testing.T, env *testhelper.TestEnv, runID int64) {
	t.Helper()
	env.EnqueueJob(t, stepexecution.RunStepArgs{RunID: runID, Step: env.DB.AutomationRun.GetX(t.Context(), runID).CurrentStep})
}

// stepOnce executes the step an Enrollment of Acme stands on, and returns what the step returned.
func stepOnce(t *testing.T, env *testhelper.TestEnv, mod *stepexecution.UseCase, runID int64) error {
	t.Helper()
	return mod.Step(t.Context(), env.DB.Scoped(fixtures.AcmeID), runID, env.DB.AutomationRun.GetX(t.Context(), runID).CurrentStep)
}

// queuedSteps are the args of the step jobs waiting in the queue.
func queuedSteps(t *testing.T, env *testhelper.TestEnv) []map[string]any {
	t.Helper()
	return env.JobsOf(t, stepexecution.RunStepArgs{}.Kind())
}

// stepJob is the args of a queued step job as JobsOf reads them.
func stepJob(runID int64, step int) map[string]any {
	return map[string]any{"run_id": float64(runID), "step": float64(step)}
}

// triggerOf is the event that enrolls a fixture Automation: its live version's Trigger, or its draft's
// when it was never published.
func triggerOf(t *testing.T, env *testhelper.TestEnv, automationID int64) string {
	t.Helper()
	ctx := context.Background()
	a := env.DB.Automation.GetX(ctx, automationID)
	versionID := a.LiveVersionID
	if versionID == nil {
		versionID = a.DraftVersionID
	}
	return env.DB.AutomationVersion.GetX(ctx, *versionID).TriggerEvent
}

// enrollVia enrolls the Contact through the Automation's own trigger and returns its one run.
func enrollVia(t *testing.T, env *testhelper.TestEnv, automationID, contactID int64) int64 {
	t.Helper()
	require.NoError(t, stepexecution.OnEvent(context.Background(), env.Bus, env.DB.Scoped(fixtures.AcmeID), contactID, automations.Event{Action: triggerOf(t, env, automationID), OccurredAt: time.Now()}, 0))
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
	require.NoError(t, stepexecution.OnEvent(ctx, env.Bus, env.DB.Scoped(fixtures.AcmeID), alice.ID, automations.Event{Action: triggerOf(t, env, fixtures.AutomationJobsGreetingID), OccurredAt: time.Now()}, 0))
	assert.Equal(t, []int64{runID}, enrollmentsOf(t, env, fixtures.AutomationJobsGreetingID, alice.ID))

	fs := &fakeSender{}
	env.RunQueuedSteps(t, stepperOf(env, resolvingTo(fs)))

	// Two email steps fired (the wait step does not send).
	require.Len(t, fs.sent, 2)
	assert.Equal(t, "Hi "+*alice.FirstName, fs.sent[0].Subject)
	assert.NotContains(t, fs.sent[0].HTML, "{{")

	assert.Equal(t, automationrun.StatusCompleted, env.DB.AutomationRun.GetX(ctx, runID).Status)

	// Each automation send publishes email.sent, so an
	// automation send is segmentable through the same Event log as broadcast sends.
	// Two email steps mean two send facts for this contact.
	sent := env.QueuedCount(t, "email.sent", map[string]any{"email": *alice.Email})
	assert.Equal(t, 2, sent)
}

// An opt-out from this Automation's own source exits the Enrollment instead of sending: an
// unsubscribe from the Automation ends it, whichever Channel it came from (ADR 0081). Leo opted out
// of the Welcome series' own sending source.
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
	queueStep(t, env, fixtures.AutomationRunJobsSingleEmailLiamID)

	fs := &fakeSender{}
	mod := outboundmessages.New(env.Bus, env.Holds, resolvingTo(fs), messagetoken.New("secret", "https://app.test"))
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

	require.NoError(t, stepexecution.OnEvent(ctx, env.Bus, env.DB.Scoped(fixtures.AcmeID), fixtures.ContactAliceID, automations.Event{Action: triggerOf(t, env, fixtures.AutomationWinBackID), OccurredAt: time.Now()}, 0))
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
	assert.WithinDuration(t, *run.ResumeAt, env.JobSchedules(t, stepexecution.RunStepArgs{}.Kind())[0], time.Second, "for the resume time")
	assert.Empty(t, fs.sent)
}

// The Instance times a wait on the Instance clock, not the wall: a year ahead of the wall, the
// Enrollment's one-hour wait ends an hour after that clock's time.
func TestTheInstanceTimesAWaitOnTheInstanceClock(t *testing.T) {
	at := time.Now().AddDate(1, 0, 0)
	env := testhelper.Setup(t, testhelper.WithClock(func() time.Time { return at }))
	runID := enrollVia(t, env, fixtures.AutomationJobsWaitFirstID, fixtures.ContactAliceID)
	want := at.Add(time.Hour)

	ran := env.RunNextStep(t, env.StepExecution)

	require.True(t, ran)
	run := env.DB.AutomationRun.GetX(t.Context(), runID)
	require.NotNil(t, run.ResumeAt)
	assert.WithinDuration(t, want, *run.ResumeAt, time.Microsecond, "an hour after the Instance clock's time")
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
	assert.WithinDuration(t, *run.ResumeAt, env.JobSchedules(t, stepexecution.RunStepArgs{}.Kind())[0], time.Second)
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
	runID := int64(fixtures.AutomationRunJobsSystemTagAliceID)
	queueStep(t, env, runID)
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
	runID := int64(fixtures.AutomationRunJobsSingleEmailLiamID)
	queueStep(t, env, runID)
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
		_, err := automations.Exit(ctx, sc, env.Publisher(), run.AutomationID, run.ContactID, time.Now())
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

// An Enrollment finishes on the steps it began with, however the Automation is edited and
// published after (ADR 0049, ADR 0065): the next Enrollment begins on the new steps.
func TestAutomationEnrollmentFinishesOnItsOwnVersionAfterAnEdit(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	mod := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule())

	early := enrollVia(t, env, fixtures.AutomationJobsEditableID, fixtures.ContactAliceID)
	// The first step (the wait) runs; the Enrollment now stands on the email step.
	fs := &fakeSender{}
	stepper := stepperOf(env, resolvingTo(fs))
	require.True(t, env.RunNextStep(t, stepper))

	// The owner edits the steps: a wait is inserted ahead and the email is reworded. Were the
	// Enrollment to read the live steps, its index 1 would now be the new wait.
	_, err := mod.Update(ctx, acme, fixtures.AutomationJobsEditableID, automations.UpdateInput{Steps: &[]automationstep.Step{
		{Type: automationstep.Wait, Seconds: 1},
		{Type: automationstep.Wait, Seconds: 2},
		{Type: automationstep.Email, Subject: "New steps", Body: "<p>new</p>"},
	}})
	require.NoError(t, err)
	_, err = mod.Publish(ctx, acme, fixtures.AutomationJobsEditableID)
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

	require.NoError(t, stepexecution.OnEvent(ctx, env.Bus, acme, fixtures.ContactAliceID, automations.Event{Action: events.NameEmailOpened, OccurredAt: time.Now()}, fixtures.AutomationOnOpenID))
	assert.Empty(t, enrollmentsOf(t, env, fixtures.AutomationOnOpenID, fixtures.ContactAliceID), "the open of its own email enrolls nobody")

	require.NoError(t, stepexecution.OnEvent(ctx, env.Bus, acme, fixtures.ContactAliceID, automations.Event{Action: events.NameEmailOpened, OccurredAt: time.Now()}, fixtures.AutomationWelcomeSeriesID))
	assert.Len(t, enrollmentsOf(t, env, fixtures.AutomationOnOpenID, fixtures.ContactAliceID), 1, "the open of another Automation's email starts On open")
}

// The goal event ends the Enrollment before its next step, which then sends nothing (#751). The
// payment occurred after the failure but before the job that enrolled was run, and still counts:
// the Enrollment began when the failure occurred. A click or other Event from the Automation's own
// email counts too: it ends, it only never enrolls.
func TestAGoalEventEndsTheEnrollmentBeforeItsNextStep(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	failed := time.Now().Add(-2 * time.Minute)
	paid := failed.Add(time.Minute)
	require.NoError(t, stepexecution.OnEvent(ctx, env.Bus, acme, fixtures.ContactAliceID, automations.Event{Action: "payment_failed", OccurredAt: failed}, 0))
	enrolled := enrollmentsOf(t, env, fixtures.AutomationPaymentRecoveryID, fixtures.ContactAliceID)
	require.Len(t, enrolled, 1)

	require.NoError(t, stepexecution.OnEvent(ctx, env.Bus, acme, fixtures.ContactAliceID, automations.Event{Action: "payment_succeeded", OccurredAt: paid}, fixtures.AutomationPaymentRecoveryID))
	fs := &fakeSender{}
	env.RunQueuedSteps(t, stepperOf(env, resolvingTo(fs)))

	assert.Empty(t, fs.sent, "no step runs after the goal")
	run, err := env.DB.AutomationRun.Get(ctx, enrolled[0])
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusGoalReached, run.Status)
}

// The goal Event's job ran before the trigger's (out of order): it found no Enrollment to end. The
// trigger's job then makes the Enrollment already at its goal, from the stored Event, and queues
// no step, so nothing is sent.
func TestAGoalEventHandledBeforeTheTriggerEndsTheEnrollmentWhenItIsMade(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	contactID := int64(fixtures.ContactPaidBeforeEnrolledID)
	failed := time.Now().Add(-4 * 24 * time.Hour) // the fixture's payment_succeeded is 3 days ago

	require.NoError(t, stepexecution.OnEvent(ctx, env.Bus, acme, contactID, automations.Event{Action: "payment_succeeded", OccurredAt: failed.Add(24 * time.Hour)}, 0))
	require.NoError(t, stepexecution.OnEvent(ctx, env.Bus, acme, contactID, automations.Event{Action: "payment_failed", OccurredAt: failed}, 0))
	fs := &fakeSender{}
	env.RunQueuedSteps(t, stepperOf(env, resolvingTo(fs)))

	assert.Empty(t, fs.sent)
	runs := enrollmentsOf(t, env, fixtures.AutomationPaymentRecoveryID, contactID)
	require.Len(t, runs, 1)
	run, err := env.DB.AutomationRun.Get(ctx, runs[0])
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusGoalReached, run.Status)
}

// A trigger job queued by a release before #751 carries no occurred_at (ADR 0033): the Event is
// taken as occurring now, so the Enrollment does not begin in year 1, and the Contact's exit and
// goal Events of the days before do not end it as it is made.
func TestATriggerJobWithoutWhenTheEventOccurredBeginsTheEnrollmentNow(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	earliest := time.Now().Add(-time.Microsecond) // began_at is kept to the microsecond

	require.NoError(t, stepexecution.OnEvent(ctx, env.Bus, acme, fixtures.ContactPaidBeforeEnrolledID, automations.Event{Action: "payment_failed", OccurredAt: time.Time{}}, 0))
	latest := time.Now()
	runs := enrollmentsOf(t, env, fixtures.AutomationPaymentRecoveryID, fixtures.ContactPaidBeforeEnrolledID)
	require.Len(t, runs, 1)
	run, err := env.DB.AutomationRun.Get(ctx, runs[0])
	require.NoError(t, err)

	assert.Equal(t, automationrun.StatusActive, run.Status)
	assert.WithinRange(t, run.BeganAt, earliest, latest)
}
