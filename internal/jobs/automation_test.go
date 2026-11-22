package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationsteprecord"
	"github.com/getsphericon/sphericon/ent/tag"
	"github.com/getsphericon/sphericon/internal/domain/sending/holds"
	"github.com/getsphericon/sphericon/internal/domain/sending/messagetoken"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
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

// stepperOf is the Step execution over the sender resolver, its clock passing every wait.
func stepperOf(env *testhelper.TestEnv, resolver outboundmessages.Senders) *automations.StepExecution {
	return stepperAt(env, newMod(env, resolver, time.Now), passingClock())
}

// stepperWith is the Step execution over mod, on the wall clock.
func stepperWith(env *testhelper.TestEnv, mod *outboundmessages.Module) *automations.StepExecution {
	return stepperAt(env, mod, time.Now)
}

// stepperAt is the Step execution over mod, on the clock now.
func stepperAt(env *testhelper.TestEnv, mod *outboundmessages.Module, now func() time.Time) *automations.StepExecution {
	return automations.NewStepExecution(env.Bus, mod, testhelper.Module[*contacts.Module](env), noFence, env.Cipher, nil, now)
}

// queueStep queues the step job of an Enrollment standing on a fixture, as the enroll or the
// previous step would have.
func queueStep(t *testing.T, env *testhelper.TestEnv, runID int64) {
	t.Helper()
	run := env.DB.AutomationRun.GetX(t.Context(), runID)
	env.EnqueueJob(t, automations.RunStepArgs{WorkspaceID: run.WorkspaceID, RunID: runID, Step: lo.FromPtr(run.CurrentStepID)})
}

// stepOnce executes the step an Enrollment of Acme stands on, and returns what the step returned.
func stepOnce(t *testing.T, env *testhelper.TestEnv, mod *automations.StepExecution, runID int64) error {
	t.Helper()
	return mod.Step(t.Context(), env.DB.Scoped(fixtures.AcmeID), runID, env.CurrentStepID(t, runID))
}

// queuedSteps are the args of the step jobs waiting in the queue.
func queuedSteps(t *testing.T, env *testhelper.TestEnv) []map[string]any {
	t.Helper()
	return env.JobsOf(t, automations.RunStepArgs{}.Kind())
}

// stepJob is the args of a queued step job of the Enrollment runID as JobsOf reads them, its
// Workspace included: the job of the Step at pos of the Enrollment's version ("" past the last).
func stepJob(t *testing.T, env *testhelper.TestEnv, runID int64, pos int) map[string]any {
	t.Helper()
	run := env.DB.AutomationRun.GetX(t.Context(), runID)
	return map[string]any{"run_id": float64(runID), "step_id": env.StepIDAt(t, runID, pos), jobkind.WorkspaceArg: float64(run.WorkspaceID)}
}

// stepAt is the args of the queued job of the Step the Enrollment runID stands on now.
func stepAt(t *testing.T, env *testhelper.TestEnv, runID int64) map[string]any {
	t.Helper()
	run := env.DB.AutomationRun.GetX(t.Context(), runID)
	return map[string]any{"run_id": float64(runID), "step_id": lo.FromPtr(run.CurrentStepID), jobkind.WorkspaceArg: float64(run.WorkspaceID)}
}

// stepRecordsOf is what the Enrollment runID of Acme kept of the Steps it reached, read through the
// Automations module, in the order it reached them.
func stepRecordsOf(t *testing.T, env *testhelper.TestEnv, runID int64) []*ent.AutomationStepRecord {
	t.Helper()
	run := env.DB.AutomationRun.GetX(t.Context(), runID)
	records, err := testhelper.Module[*automations.Module](env).StepRecords(t.Context(), env.DB.Scoped(run.WorkspaceID), run.AutomationID, runID)
	require.NoError(t, err)
	return records
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
	require.NoError(t, testhelper.Module[*automations.StepExecution](env).OnEvent(context.Background(), env.DB.Scoped(fixtures.AcmeID), automations.OnEventInput{ContactID: contactID, Event: automations.Event{Action: triggerOf(t, env, automationID), OccurredAt: time.Now()}}))
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
	require.NoError(t, testhelper.Module[*automations.StepExecution](env).OnEvent(ctx, env.DB.Scoped(fixtures.AcmeID), automations.OnEventInput{ContactID: alice.ID, Event: automations.Event{Action: triggerOf(t, env, fixtures.AutomationJobsGreetingID), OccurredAt: time.Now()}}))
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
	sent := env.QueuedCount(t, systemevent.EmailSent, map[string]any{"email": *alice.Email})
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
	mod := outboundmessages.New(env.Bus, testhelper.Module[*holds.Holds](env), resolvingTo(fs), messagetoken.New("secret", "https://app.test", time.Now), env.OutboundModules(env.Bus, time.Now), outboundmessages.Deps{}, outboundmessages.Config{}, time.Now)
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

	require.NoError(t, testhelper.Module[*automations.StepExecution](env).OnEvent(ctx, env.DB.Scoped(fixtures.AcmeID), automations.OnEventInput{ContactID: fixtures.ContactAliceID, Event: automations.Event{Action: triggerOf(t, env, fixtures.AutomationWinBackID), OccurredAt: time.Now()}}))
	assert.Empty(t, enrollmentsOf(t, env, fixtures.AutomationWinBackID, fixtures.ContactAliceID), "the win-back automation is a draft")
}

// An Enrollment that reaches a wait stands on the wait itself (ADR 0124): reaching it records its
// due moment, on the Enrollment and in the wait's Step record, and queues the wait's own job for
// then, and nothing else.
func TestAnEnrollmentReachingAWaitStandsOnItUntilItIsDue(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	before := time.Now()
	runID := enrollVia(t, env, fixtures.AutomationJobsWaitFirstID, fixtures.ContactAliceID)

	run := env.DB.AutomationRun.GetX(ctx, runID)
	records := stepRecordsOf(t, env, runID)
	schedules := env.JobSchedules(t, automations.RunStepArgs{}.Kind())

	require.NotNil(t, run.ResumeAt)
	assert.Equal(t, env.StepIDAt(t, runID, 0), lo.FromPtr(run.CurrentStepID), "the Enrollment stands on the wait")
	assert.WithinDuration(t, before.Add(time.Hour), *run.ResumeAt, time.Minute, "due an hour after it reached the wait")
	assert.Equal(t, []map[string]any{stepJob(t, env, runID, 0)}, queuedSteps(t, env), "the wait's own job is queued, and nothing else")
	assert.WithinDuration(t, *run.ResumeAt, schedules[0], time.Second, "for the due moment")
	require.Len(t, records, 1)
	assert.Equal(t, env.StepIDAt(t, runID, 0), records[0].StepID)
	assert.Equal(t, run.ResumeAt, records[0].DueAt, "the wait's Step record keeps its due moment")
	assert.Nil(t, records[0].LeftAt, "the Enrollment has not left the wait")
}

// The Instance times a wait on the Instance clock, not the wall: a year ahead of the wall, the
// Enrollment's one-hour wait is due an hour after that clock's time.
func TestTheInstanceTimesAWaitOnTheInstanceClock(t *testing.T) {
	at := time.Now().AddDate(1, 0, 0)
	env := testhelper.Setup(t, testhelper.WithClock(func() time.Time { return at }))
	runID := enrollVia(t, env, fixtures.AutomationJobsWaitFirstID, fixtures.ContactAliceID)
	want := at.Add(time.Hour)

	run := env.DB.AutomationRun.GetX(t.Context(), runID)

	require.NotNil(t, run.ResumeAt)
	assert.WithinDuration(t, want, *run.ResumeAt, time.Microsecond, "an hour after the Instance clock's time")
}

// A wait's job that runs before the due moment (a restart, a job delivered early) queues itself
// for that moment and does nothing else: the Enrollment stays on the wait, due when it was, and
// the email after the wait is not sent early. At the due moment it moves on.
func TestAWaitRunBeforeItIsDueNeitherMovesEarlyNorWaitsTwice(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	runID := enrollVia(t, env, fixtures.AutomationJobsWaitFirstID, fixtures.ContactAliceID)
	due := env.DB.AutomationRun.GetX(ctx, runID).ResumeAt
	fs := &fakeSender{}

	require.True(t, env.RunNextStep(t, stepperWith(env, newMod(env, resolvingTo(fs), time.Now))))

	run := env.DB.AutomationRun.GetX(ctx, runID)
	records := stepRecordsOf(t, env, runID)
	assert.Empty(t, fs.sent, "the email after the wait is not sent before the due moment")
	assert.Equal(t, env.StepIDAt(t, runID, 0), lo.FromPtr(run.CurrentStepID), "the Enrollment stays on the wait")
	assert.Equal(t, automationrun.StatusActive, run.Status)
	assert.Equal(t, due, run.ResumeAt, "the due moment is not computed again")
	assert.Equal(t, []map[string]any{stepJob(t, env, runID, 0)}, queuedSteps(t, env), "the wait's job waits for its moment")
	assert.WithinDuration(t, *due, env.JobSchedules(t, automations.RunStepArgs{}.Kind())[0], time.Second)
	require.Len(t, records, 1)
	assert.Equal(t, due, records[0].DueAt)

	env.RunQueuedSteps(t, stepperOf(env, resolvingTo(fs)))

	assert.Len(t, fs.sent, 1, "past the due moment the Enrollment moves on and sends")
}

// Every Step an Enrollment passes leaves a Step record of when it reached and left it (ADR 0124),
// and leaving the last Step completes the Enrollment in the same move: it stays on that Step and
// no further job is queued.
func TestEachStepAnEnrollmentPassesHasAStepRecordAndTheLastCompletesIt(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	runID := enrollVia(t, env, fixtures.AutomationJobsGreetingID, fixtures.ContactAliceID)
	mod := stepperOf(env, resolvingTo(&fakeSender{}))

	require.True(t, env.RunNextStep(t, mod)) // the first email
	require.True(t, env.RunNextStep(t, mod)) // the wait
	require.True(t, env.RunNextStep(t, mod)) // the last email

	run := env.DB.AutomationRun.GetX(ctx, runID)
	records := stepRecordsOf(t, env, runID)
	assert.Equal(t, automationrun.StatusCompleted, run.Status, "leaving the last Step completed the Enrollment")
	assert.Equal(t, env.StepIDAt(t, runID, 2), lo.FromPtr(run.CurrentStepID), "it ended on its last Step")
	assert.Empty(t, queuedSteps(t, env), "nothing is queued after the last Step")
	assert.Equal(t, []string{env.StepIDAt(t, runID, 0), env.StepIDAt(t, runID, 1), env.StepIDAt(t, runID, 2)},
		lo.Map(records, func(r *ent.AutomationStepRecord, _ int) string { return r.StepID }))
	assert.Equal(t, []bool{true, true, true}, lo.Map(records, func(r *ent.AutomationStepRecord, _ int) bool { return r.LeftAt != nil }), "each Step was left")
	assert.Equal(t, []automationsteprecord.Outcome{automationsteprecord.OutcomeDone, automationsteprecord.OutcomeDone, automationsteprecord.OutcomeDone},
		lo.Map(records, func(r *ent.AutomationStepRecord, _ int) automationsteprecord.Outcome { return lo.FromPtr(r.Outcome) }))
	assert.Equal(t, []bool{false, true, false}, lo.Map(records, func(r *ent.AutomationStepRecord, _ int) bool { return r.DueAt != nil }), "only the wait has a due moment")
	assert.False(t, records[1].ReachedAt.Before(*records[0].LeftAt), "the wait was reached when the first email was left")
}

// An Enrollment on a version of no Steps has nothing to run: it completes as it begins, and no job
// is queued for it.
func TestAnEnrollmentOnAVersionOfNoStepsCompletesAsItBegins(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	mod := testhelper.Module[*automations.Module](env)
	_, err := mod.Update(ctx, acme, fixtures.AutomationJobsEditableID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, acme, fixtures.AutomationJobsEditableID), Steps: new(automationstep.Root())})
	require.NoError(t, err)
	_, err = mod.Publish(ctx, acme, fixtures.AutomationJobsEditableID, testhelper.AutomationRevisionIn(ctx, t, acme, fixtures.AutomationJobsEditableID))
	require.NoError(t, err)

	runID := enrollVia(t, env, fixtures.AutomationJobsEditableID, fixtures.ContactCarolID)

	run := env.DB.AutomationRun.GetX(ctx, runID)
	assert.Equal(t, automationrun.StatusCompleted, run.Status)
	assert.Nil(t, run.CurrentStepID, "it stands on no Step")
	assert.Empty(t, queuedSteps(t, env))
}

// A step job delivered again after its transaction committed (the queue completes a job in a
// separate step) finds the Enrollment on the next step: it does nothing and queues nothing.
func TestAStepJobExecutedTwiceMovesTheEnrollmentOnce(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	runID := enrollVia(t, env, fixtures.AutomationJobsWaitFirstID, fixtures.ContactAliceID)
	mod := stepperOf(env, resolvingTo(&fakeSender{}))
	acme := env.DB.Scoped(fixtures.AcmeID)

	require.NoError(t, mod.Step(ctx, acme, runID, env.StepIDAt(t, runID, 0)))
	afterFirst := queuedSteps(t, env)
	runAfterFirst := env.DB.AutomationRun.GetX(ctx, runID)

	require.NoError(t, mod.Step(ctx, acme, runID, env.StepIDAt(t, runID, 0)))

	assert.Equal(t, afterFirst, queuedSteps(t, env), "the second execution queues nothing")
	runAfterSecond := env.DB.AutomationRun.GetX(ctx, runID)
	assert.Equal(t, env.StepIDAt(t, runID, 1), lo.FromPtr(runAfterSecond.CurrentStepID), "the Enrollment moved once")
	assert.Equal(t, runAfterFirst.UpdatedAt, runAfterSecond.UpdatedAt, "the second execution wrote nothing")
	assert.Equal(t, runAfterFirst.ResumeAt, runAfterSecond.ResumeAt)
}

// A tag step delivered twice tags once and moves once: it is its version's last Step, so the
// first execution completes the Enrollment and neither queues a follow-up.
func TestATagStepJobExecutedTwiceMovesOnce(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	runID := int64(fixtures.AutomationRunJobsSystemTagAliceID)
	queueStep(t, env, runID)
	mod := stepperOf(env, resolvingTo(&fakeSender{}))
	acme := env.DB.Scoped(fixtures.AcmeID)

	require.NoError(t, mod.Step(ctx, acme, runID, env.StepIDAt(t, runID, 0)))
	require.NoError(t, mod.Step(ctx, acme, runID, env.StepIDAt(t, runID, 0)))

	assert.Equal(t, []map[string]any{stepJob(t, env, runID, 0)}, queuedSteps(t, env), "the enrolling job and no follow-up")
	assert.Equal(t, automationrun.StatusCompleted, env.DB.AutomationRun.GetX(ctx, runID).Status)
	assert.Equal(t, env.StepIDAt(t, runID, 0), env.CurrentStepID(t, runID))
}

// A Contact erased after its step was queued takes the Enrollment with it: the step finds
// nothing, ends quietly and sends nothing.
func TestAStepOfAnEnrollmentErasedWithItsContactEndsQuietly(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	runID := int64(fixtures.AutomationRunJobsSingleEmailLiamID)
	queueStep(t, env, runID)
	env.Feed(t, func(tx *ent.Client) error { return tx.Contact.DeleteOneID(fixtures.ContactLiamID).Exec(ctx) })
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
	assert.Equal(t, env.StepIDAt(t, runID, 0), lo.FromPtr(run.CurrentStepID), "a hold does not advance the enrollment")
	assert.Equal(t, []map[string]any{stepJob(t, env, runID, 0)}, queuedSteps(t, env), "the worker reschedules the same step")
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
		_, err := env.ExitEnrollment(ctx, sc, run.AutomationID, run.ContactID)
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
	assert.Equal(t, env.StepIDAt(t, runID, 0), lo.FromPtr(run.CurrentStepID))
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
	assert.Equal(t, env.StepIDAt(t, runID, 0), env.CurrentStepID(t, runID))
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
	assert.Equal(t, env.StepIDAt(t, runID, 1), lo.FromPtr(run.CurrentStepID))
	assert.Empty(t, queuedSteps(t, env))
}

// An Enrollment finishes on the steps it began with, however the Automation is edited and
// published after (ADR 0049, ADR 0065): the next Enrollment begins on the new steps.
func TestAutomationEnrollmentFinishesOnItsOwnVersionAfterAnEdit(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	mod := testhelper.Module[*automations.Module](env)

	early := enrollVia(t, env, fixtures.AutomationJobsEditableID, fixtures.ContactAliceID)
	// The first step (the wait) runs; the Enrollment now stands on the email step.
	fs := &fakeSender{}
	stepper := stepperOf(env, resolvingTo(fs))
	require.True(t, env.RunNextStep(t, stepper))

	// The owner edits the steps: a wait is inserted ahead and the email is reworded. Were the
	// Enrollment to read the live steps, its index 1 would now be the new wait.
	_, err := mod.Update(ctx, acme, fixtures.AutomationJobsEditableID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, acme, fixtures.AutomationJobsEditableID), Steps: new(automationstep.Root(
		automationstep.New(automationstep.Wait{Seconds: 1}),
		automationstep.New(automationstep.Wait{Seconds: 2}),
		automationstep.New(automationstep.Email{Subject: "New steps", Body: "<p>new</p>"}),
	))})
	require.NoError(t, err)
	_, err = mod.Publish(ctx, acme, fixtures.AutomationJobsEditableID, testhelper.AutomationRevisionIn(ctx, t, acme, fixtures.AutomationJobsEditableID))
	require.NoError(t, err)

	late := enrollVia(t, env, fixtures.AutomationJobsEditableID, fixtures.ContactCarolID)

	env.RunQueuedSteps(t, stepper)

	require.Len(t, fs.sent, 2)
	assert.Equal(t, "Old steps", fs.sent[0].Subject, "the Enrollment begun before the edit finished on its own version")
	assert.Equal(t, "New steps", fs.sent[1].Subject, "the Enrollment begun after the edit used the new steps")
	assert.Equal(t, automationrun.StatusCompleted, env.DB.AutomationRun.GetX(ctx, early).Status)
	assert.NotEqual(t, env.DB.AutomationRun.GetX(ctx, early).VersionID, env.DB.AutomationRun.GetX(ctx, late).VersionID)
}

// An Enrollment names the Step it stands on by its id (ADR 0122): a new version that inserts a
// Step above that one, keeping the ids of the rest, leaves the Enrollment on the same Step of its
// own version, and the inserted Step is never run for it.
func TestPublishingAStepInsertedAboveLeavesTheEnrollmentOnItsStep(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	mod := testhelper.Module[*automations.Module](env)
	runID := enrollVia(t, env, fixtures.AutomationJobsEditableID, fixtures.ContactAliceID)
	fs := &fakeSender{}
	stepper := stepperOf(env, resolvingTo(fs))
	require.True(t, env.RunNextStep(t, stepper)) // the wait
	live := testhelper.LatestSteps(ctx, t, env.DB, fixtures.AutomationJobsEditableID)
	wait, email := live.Steps[0], live.Steps[1]
	require.Equal(t, email.ID, lo.FromPtr(env.DB.AutomationRun.GetX(ctx, runID).CurrentStepID), "the Enrollment stands on the email")

	_, err := mod.Update(ctx, acme, fixtures.AutomationJobsEditableID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, acme, fixtures.AutomationJobsEditableID), Steps: &automationstep.Path{ID: live.ID, Steps: []automationstep.Step{
		wait,
		automationstep.New(automationstep.ApplyTag{Tag: "inserted"}),
		email,
	}}})
	require.NoError(t, err)
	_, err = mod.Publish(ctx, acme, fixtures.AutomationJobsEditableID, testhelper.AutomationRevisionIn(ctx, t, acme, fixtures.AutomationJobsEditableID))
	require.NoError(t, err)

	run := env.DB.AutomationRun.GetX(ctx, runID)
	assert.Equal(t, email.ID, lo.FromPtr(run.CurrentStepID), "the Enrollment stays on its Step")
	env.RunQueuedSteps(t, stepper)

	require.Len(t, fs.sent, 1)
	assert.Equal(t, "Old steps", fs.sent[0].Subject)
	assert.Equal(t, automationrun.StatusCompleted, env.DB.AutomationRun.GetX(ctx, runID).Status)
	assert.False(t, env.DB.Tag.Query().Where(tag.Name("inserted")).ExistX(ctx), "the Step inserted above is not run for it")
}

// An Event from an Automation's own email never enrolls the Contact into that Automation, so an
// `email.opened` Automation cannot be fed by the mail it sends; another Automation on the same
// trigger still enrolls (ADR 0053).
func TestAnAutomationIsNotStartedByItsOwnEmail(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)

	require.NoError(t, testhelper.Module[*automations.StepExecution](env).OnEvent(ctx, acme, automations.OnEventInput{ContactID: fixtures.ContactAliceID, Event: automations.Event{Action: systemevent.EmailOpened, OccurredAt: time.Now()}, SourceAutomationID: fixtures.AutomationOnOpenID}))
	assert.Empty(t, enrollmentsOf(t, env, fixtures.AutomationOnOpenID, fixtures.ContactAliceID), "the open of its own email enrolls nobody")

	require.NoError(t, testhelper.Module[*automations.StepExecution](env).OnEvent(ctx, acme, automations.OnEventInput{ContactID: fixtures.ContactAliceID, Event: automations.Event{Action: systemevent.EmailOpened, OccurredAt: time.Now()}, SourceAutomationID: fixtures.AutomationWelcomeSeriesID}))
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
	require.NoError(t, testhelper.Module[*automations.StepExecution](env).OnEvent(ctx, acme, automations.OnEventInput{ContactID: fixtures.ContactAliceID, Event: automations.Event{Action: "payment_failed", OccurredAt: failed}}))
	enrolled := enrollmentsOf(t, env, fixtures.AutomationPaymentRecoveryID, fixtures.ContactAliceID)
	require.Len(t, enrolled, 1)

	require.NoError(t, testhelper.Module[*automations.StepExecution](env).OnEvent(ctx, acme, automations.OnEventInput{ContactID: fixtures.ContactAliceID, Event: automations.Event{Action: "payment_succeeded", OccurredAt: paid}, SourceAutomationID: fixtures.AutomationPaymentRecoveryID}))
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

	require.NoError(t, testhelper.Module[*automations.StepExecution](env).OnEvent(ctx, acme, automations.OnEventInput{ContactID: contactID, Event: automations.Event{Action: "payment_succeeded", OccurredAt: failed.Add(24 * time.Hour)}}))
	require.NoError(t, testhelper.Module[*automations.StepExecution](env).OnEvent(ctx, acme, automations.OnEventInput{ContactID: contactID, Event: automations.Event{Action: "payment_failed", OccurredAt: failed}}))
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

	require.NoError(t, testhelper.Module[*automations.StepExecution](env).OnEvent(ctx, acme, automations.OnEventInput{ContactID: fixtures.ContactPaidBeforeEnrolledID, Event: automations.Event{Action: "payment_failed", OccurredAt: time.Time{}}}))
	latest := time.Now()
	runs := enrollmentsOf(t, env, fixtures.AutomationPaymentRecoveryID, fixtures.ContactPaidBeforeEnrolledID)
	require.Len(t, runs, 1)
	run, err := env.DB.AutomationRun.Get(ctx, runs[0])
	require.NoError(t, err)

	assert.Equal(t, automationrun.StatusActive, run.Status)
	assert.WithinRange(t, run.BeganAt, earliest, latest)
}
