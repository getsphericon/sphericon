package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/messaging"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/tag"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/tracking"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// drive runs RunStep until the run finishes (or a safety cap). tracker may be nil
// (most tests don't assert on the unsubscribe footer).
func drive(t *testing.T, env *testhelper.TestEnv, resolver outbound.Senders, runID int64) {
	t.Helper()
	ctx := context.Background()
	for range 20 {
		res, err := jobs.RunStep(ctx, env.DB, noFence, newMod(env, resolver), runID)
		require.NoError(t, err)
		if res.Done {
			return
		}
	}
	t.Fatal("run did not finish within cap")
}

// triggerOf is the event that enrolls a fixture Automation.
func triggerOf(t *testing.T, env *testhelper.TestEnv, automationID int64) string {
	t.Helper()
	return env.DB.Automation.GetX(context.Background(), automationID).TriggerEvent
}

// enrollVia enrolls the Contact through the Automation's own trigger and returns its one run.
func enrollVia(t *testing.T, env *testhelper.TestEnv, automationID, contactID int64) int64 {
	t.Helper()
	runIDs, err := jobs.EvaluateTrigger(context.Background(), env.DB.Scoped(fixtures.AcmeID), contactID, triggerOf(t, env, automationID), 0)
	require.NoError(t, err)
	require.Len(t, runIDs, 1)
	return runIDs[0]
}

func TestAutomationEnrollAndRun(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	alice := env.DB.Contact.GetX(ctx, fixtures.ContactAliceID)
	runID := enrollVia(t, env, fixtures.AutomationJobsGreetingID, alice.ID)

	// Enroll-once: a second matching event creates no new run.
	again, err := jobs.EvaluateTrigger(ctx, env.DB.Scoped(fixtures.AcmeID), alice.ID, triggerOf(t, env, fixtures.AutomationJobsGreetingID), 0)
	require.NoError(t, err)
	assert.Empty(t, again)

	fs := &fakeSender{}
	drive(t, env, resolvingTo(fs), runID)

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
	drive(t, env, resolvingTo(fs), runID)

	assert.Empty(t, fs.sent, "ineligible destination receives no email")
	assert.Equal(t, automationrun.StatusExited, env.DB.AutomationRun.GetX(ctx, runID).Status)
}

// A suppression exits the enrollment too (the global hard floor).
func TestAutomationExitsOnSuppression(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	runID := enrollVia(t, env, fixtures.AutomationJobsSingleEmailID, fixtures.ContactComplainedID)

	fs := &fakeSender{}
	drive(t, env, resolvingTo(fs), runID)

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
	drive(t, env, resolvingTo(fs), runID)

	assert.Len(t, fs.sent, 1, "broadcasts opt-out must not block the automation")
	assert.Equal(t, automationrun.StatusCompleted, env.DB.AutomationRun.GetX(ctx, runID).Status)
}

// An automation send carries an unsubscribe footer scoped to the automation, so
// the recipient can opt out (which exits the enrollment: see the endpoint test).
func TestAutomationSendIncludesUnsubscribeFooter(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	runID := enrollVia(t, env, fixtures.AutomationJobsSingleEmailID, fixtures.ContactLiamID)

	fs := &fakeSender{}
	mod := outbound.New(env.Bus, env.Holds, resolvingTo(fs), tracking.New("secret", "https://app.test"))
	_, err := jobs.RunStep(ctx, env.DB, noFence, mod, runID)
	require.NoError(t, err)

	require.Len(t, fs.sent, 1)
	assert.Contains(t, fs.sent[0].HTML, "https://app.test/e/u/", "automation email carries an unsubscribe link")
	// RFC 8058 one-click header (ADR 0012), scoped to this automation source.
	assert.Contains(t, fs.sent[0].ListUnsubscribeURL, "https://app.test/e/u/",
		"automation sets a one-click unsubscribe URL")
}

func TestAutomationInactiveDoesNotEnroll(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	runIDs, err := jobs.EvaluateTrigger(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.ContactAliceID, triggerOf(t, env, fixtures.AutomationWinBackID), 0)
	require.NoError(t, err)
	assert.Empty(t, runIDs, "the win-back automation is a draft")
}

func TestAutomationWaitDefersNextStep(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	runID := enrollVia(t, env, fixtures.AutomationJobsWaitFirstID, fixtures.ContactAliceID)

	// First step is a wait: it defers (ResumeAt set), no send yet.
	fs := &fakeSender{}
	res, err := jobs.RunStep(ctx, env.DB, noFence, newMod(env, resolvingTo(fs)), runID)
	require.NoError(t, err)
	assert.False(t, res.Done)
	require.NotNil(t, res.ResumeAt)
	assert.NotNil(t, env.DB.AutomationRun.GetX(ctx, runID).ResumeAt)
	assert.Empty(t, fs.sent)
}

// A hold on the source (here: a suspended Workspace) leaves the enrollment exactly
// where it was and asks to be run again later; the run is neither failed nor exited.
func TestAutomationHeldStepWaitsAndResumes(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	runID := enrollVia(t, env, fixtures.AutomationHoldDemoID, fixtures.ContactHoldDemoID)

	fs := &fakeSender{}
	mod := newMod(env, resolvingTo(fs))
	env.SuspendWorkspace(ctx, fixtures.AcmeID, time.Now())

	res, err := jobs.RunStep(ctx, env.DB, noFence, mod, runID)
	require.NoError(t, err)
	assert.False(t, res.Done)
	require.NotNil(t, res.ResumeAt, "the worker reschedules the same step")
	run := env.DB.AutomationRun.GetX(ctx, runID)
	assert.Equal(t, automationrun.StatusActive, run.Status)
	assert.Zero(t, run.CurrentStep, "a hold does not advance the enrollment")
	assert.Empty(t, fs.sent)

	env.UnsuspendWorkspace(ctx, fixtures.AcmeID)
	drive(t, env, resolvingTo(fs), runID)
	assert.Len(t, fs.sent, 1, "after the unfreeze the step sends")
}

// An Enrollment exited while a step is in flight stays exited: the late step's own
// outcome (here a send that can never be composed) finishes quietly instead of
// overwriting the exit.
func TestAutomationExitedMidStepStaysExitedAndTheLateStepFinishesQuietly(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	run := env.DB.AutomationRun.GetX(ctx, fixtures.AutomationRunJobsBrokenTemplateID)

	// The Contact unsubscribes while the step is already running: the provider lookup
	// is inside the step, after the run was read.
	resolver := &SendersMock{EmailSenderFunc: func(ctx context.Context, sc *ent.Scoped) (messaging.EmailSender, error) {
		_, err := automations.Exit(ctx, sc, run.AutomationID, run.ContactID)
		require.NoError(t, err)
		return &fakeSender{}, nil
	}}

	res, err := jobs.RunStep(ctx, env.DB, noFence, newMod(env, resolver), run.ID)
	require.NoError(t, err)
	assert.True(t, res.Done)
	assert.Equal(t, automationrun.StatusExited, env.DB.AutomationRun.GetX(ctx, run.ID).Status)
}

func TestStepOnAnExitedRunSendsNothingAndStaysPut(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	fs := &fakeSender{}
	runID := int64(fixtures.AutomationRunJobsExitedOnEmailID)

	res, err := jobs.RunStep(ctx, env.DB, noFence, newMod(env, resolvingTo(fs)), runID)

	require.NoError(t, err)
	assert.True(t, res.Done)
	assert.Empty(t, fs.sent, "nothing is sent after an exit")
	run := env.DB.AutomationRun.GetX(ctx, runID)
	assert.Equal(t, automationrun.StatusExited, run.Status)
	assert.Equal(t, 0, run.CurrentStep)
}

func TestTagStepOnAnExitedRunChangesNoTagAndDoesNotAdvance(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	runID := int64(fixtures.AutomationRunJobsExitedOnTagID)
	run := env.DB.AutomationRun.GetX(ctx, runID)

	res, err := jobs.RunStep(ctx, env.DB, noFence, newMod(env, resolvingTo(&fakeSender{})), runID)

	require.NoError(t, err)
	assert.True(t, res.Done)
	assert.False(t, env.DB.Contact.GetX(ctx, run.ContactID).QueryTags().Where(tag.Name("system-made")).ExistX(ctx), "the exited Contact was not tagged")
	assert.Equal(t, 0, env.DB.AutomationRun.GetX(ctx, runID).CurrentStep)
}

func TestWaitStepOnAnExitedRunDoesNotSetResumeAt(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	runID := int64(fixtures.AutomationRunJobsExitedOnWaitID) // exited with the wait step next

	res, err := jobs.RunStep(ctx, env.DB, noFence, newMod(env, resolvingTo(&fakeSender{})), runID)

	require.NoError(t, err)
	assert.True(t, res.Done)
	assert.Nil(t, res.ResumeAt)
	run := env.DB.AutomationRun.GetX(ctx, runID)
	assert.Equal(t, automationrun.StatusExited, run.Status)
	assert.Nil(t, run.ResumeAt)
	assert.Equal(t, 1, run.CurrentStep)
}

// An Enrollment finishes on the steps it began with, however the Automation is edited after
// (ADR 0049): the next Enrollment begins on the new steps.
func TestAutomationEnrollmentFinishesOnItsOwnVersionAfterAnEdit(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	mod := automations.New(env.Cipher, env.Bus)

	early := enrollVia(t, env, fixtures.AutomationJobsEditableID, fixtures.ContactAliceID)
	// The first step (the wait) runs; the Enrollment now stands on the email step.
	fs := &fakeSender{}
	_, err := jobs.RunStep(ctx, env.DB, noFence, newMod(env, resolvingTo(fs)), early)
	require.NoError(t, err)

	// The owner edits the steps: a wait is inserted ahead and the email is reworded. Were the
	// Enrollment to read the live steps, its index 1 would now be the new wait.
	_, err = mod.Update(ctx, acme, fixtures.AutomationJobsEditableID, automations.UpdateInput{Steps: &[]automations.Step{
		{Type: automations.StepWait, Seconds: 1},
		{Type: automations.StepWait, Seconds: 2},
		{Type: automations.StepEmail, Subject: "New steps", Body: "<p>new</p>"},
	}})
	require.NoError(t, err)

	late := enrollVia(t, env, fixtures.AutomationJobsEditableID, fixtures.ContactCarolID)

	drive(t, env, resolvingTo(fs), early)
	drive(t, env, resolvingTo(fs), late)

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

	own, err := jobs.EvaluateTrigger(ctx, acme, fixtures.ContactAliceID, events.NameEmailOpened, fixtures.AutomationOnOpenID)
	require.NoError(t, err)
	assert.Empty(t, own, "the open of its own email enrolls nobody")

	other, err := jobs.EvaluateTrigger(ctx, acme, fixtures.ContactAliceID, events.NameEmailOpened, fixtures.AutomationWelcomeSeriesID)
	require.NoError(t, err)
	assert.Len(t, other, 1, "the open of another Automation's email starts On open")
}
