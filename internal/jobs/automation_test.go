package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/messaging"

	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/tracking"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const emailStep = `{"type":"email","subject":"Hi {{ first_name }}","body":"<p>Hi {{ first_name }}</p>"}`

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

func TestAutomationEnrollAndRun(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	c, err := env.DB.Contact.Create().SetWorkspaceID(fixtures.AcmeID).
		SetEmail("auto@test.dev").SetFirstName("Ada").Save(ctx)
	require.NoError(t, err)

	_, err = testhelper.AutomationWithSteps(ctx, env.DB, env.DB.Automation.Create().SetWorkspaceID(fixtures.AcmeID).
		SetName("Welcome").SetTriggerEvent("contact.created").SetStatus(automation.StatusActive), "["+emailStep+`,{"type":"wait","seconds":3600},`+emailStep+"]")
	require.NoError(t, err)

	runIDs, err := jobs.EvaluateTrigger(ctx, env.DB.Scoped(fixtures.AcmeID), c.ID, "contact.created")
	require.NoError(t, err)
	require.Len(t, runIDs, 1)

	// Enroll-once: a second matching event creates no new run.
	again, err := jobs.EvaluateTrigger(ctx, env.DB.Scoped(fixtures.AcmeID), c.ID, "contact.created")
	require.NoError(t, err)
	assert.Empty(t, again)

	fs := &fakeSender{}
	drive(t, env, resolvingTo(fs), runIDs[0])

	// Two email steps fired (the wait step does not send).
	assert.Len(t, fs.sent, 2)
	assert.Equal(t, "Hi Ada", fs.sent[0].Subject)
	assert.NotContains(t, fs.sent[0].HTML, "{{")

	got := env.DB.AutomationRun.GetX(ctx, runIDs[0])
	assert.Equal(t, automationrun.StatusCompleted, got.Status)

	// Each automation send publishes email.sent onto the transactional outbox, so an
	// automation send is segmentable through the same Event log as broadcast sends.
	// Two email steps ⇒ two send facts for this contact.
	sent := env.OutboxCount(t, "email.sent", map[string]any{"email": "auto@test.dev"})
	assert.Equal(t, 2, sent)
}

// An opt-out from this automation's source (or a suppression) exits the
// enrollment instead of sending — a run never silently skips its emails.
func TestAutomationExitsOnUnsubscribe(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	c, err := env.DB.Contact.Create().SetWorkspaceID(fixtures.AcmeID).
		SetEmail("optout@test.dev").SetFirstName("Opt").Save(ctx)
	require.NoError(t, err)

	a, err := testhelper.AutomationWithSteps(ctx, env.DB, env.DB.Automation.Create().SetWorkspaceID(fixtures.AcmeID).
		SetName("Welcome").SetTriggerEvent("contact.created").SetStatus(automation.StatusActive), "["+emailStep+"]")
	require.NoError(t, err)

	// Opt the destination out of THIS automation's sending source.
	_, err = env.DB.Unsubscribe.Create().
		SetWorkspaceID(fixtures.AcmeID).
		SetChannel(unsubscribe.ChannelEmail).
		SetDestination(*c.Email).
		SetSendingSource(eligibility.AutomationSource(a.ID)).
		SetContactID(c.ID).
		Save(ctx)
	require.NoError(t, err)

	runIDs, err := jobs.EvaluateTrigger(ctx, env.DB.Scoped(fixtures.AcmeID), c.ID, "contact.created")
	require.NoError(t, err)
	require.Len(t, runIDs, 1)

	fs := &fakeSender{}
	drive(t, env, resolvingTo(fs), runIDs[0])

	assert.Empty(t, fs.sent, "ineligible destination receives no email")
	assert.Equal(t, automationrun.StatusExited, env.DB.AutomationRun.GetX(ctx, runIDs[0]).Status)
}

// A suppression exits the enrollment too (the global hard floor).
func TestAutomationExitsOnSuppression(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	c, err := env.DB.Contact.Create().SetWorkspaceID(fixtures.AcmeID).
		SetEmail("bounced@test.dev").SetFirstName("B").Save(ctx)
	require.NoError(t, err)
	_, err = env.DB.Suppression.Create().
		SetWorkspaceID(fixtures.AcmeID).
		SetChannel(suppression.ChannelEmail).
		SetDestination(*c.Email).
		SetReason(suppression.ReasonBounce).Save(ctx)
	require.NoError(t, err)

	a, err := testhelper.AutomationWithSteps(ctx, env.DB, env.DB.Automation.Create().SetWorkspaceID(fixtures.AcmeID).
		SetName("Welcome").SetTriggerEvent("contact.created").SetStatus(automation.StatusActive), "["+emailStep+"]")
	require.NoError(t, err)
	_ = a

	runIDs, err := jobs.EvaluateTrigger(ctx, env.DB.Scoped(fixtures.AcmeID), c.ID, "contact.created")
	require.NoError(t, err)
	require.Len(t, runIDs, 1)

	fs := &fakeSender{}
	drive(t, env, resolvingTo(fs), runIDs[0])

	assert.Empty(t, fs.sent)
	assert.Equal(t, automationrun.StatusExited, env.DB.AutomationRun.GetX(ctx, runIDs[0]).Status)
}

// A per-source opt-out is scoped: unsubscribing from "broadcasts" does not block
// an automation (a different sending source).
func TestAutomationUnaffectedByBroadcastUnsubscribe(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	c, err := env.DB.Contact.Create().SetWorkspaceID(fixtures.AcmeID).
		SetEmail("scoped@test.dev").SetFirstName("S").Save(ctx)
	require.NoError(t, err)
	_, err = env.DB.Unsubscribe.Create().
		SetWorkspaceID(fixtures.AcmeID).
		SetChannel(unsubscribe.ChannelEmail).
		SetDestination(*c.Email).
		SetSendingSource(eligibility.SourceBroadcasts).
		SetContactID(c.ID).Save(ctx)
	require.NoError(t, err)

	_, err = testhelper.AutomationWithSteps(ctx, env.DB, env.DB.Automation.Create().SetWorkspaceID(fixtures.AcmeID).
		SetName("Welcome").SetTriggerEvent("contact.created").SetStatus(automation.StatusActive), "["+emailStep+"]")
	require.NoError(t, err)

	runIDs, err := jobs.EvaluateTrigger(ctx, env.DB.Scoped(fixtures.AcmeID), c.ID, "contact.created")
	require.NoError(t, err)
	require.Len(t, runIDs, 1)

	fs := &fakeSender{}
	drive(t, env, resolvingTo(fs), runIDs[0])

	assert.Len(t, fs.sent, 1, "broadcasts opt-out must not block the automation")
	assert.Equal(t, automationrun.StatusCompleted, env.DB.AutomationRun.GetX(ctx, runIDs[0]).Status)
}

// An automation send carries an unsubscribe footer scoped to the automation, so
// the recipient can opt out (which exits the enrollment — see the endpoint test).
func TestAutomationSendIncludesUnsubscribeFooter(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	c, err := env.DB.Contact.Create().SetWorkspaceID(fixtures.AcmeID).
		SetEmail("foot@test.dev").SetFirstName("Foo").Save(ctx)
	require.NoError(t, err)
	_, err = testhelper.AutomationWithSteps(ctx, env.DB, env.DB.Automation.Create().SetWorkspaceID(fixtures.AcmeID).
		SetName("Welcome").SetTriggerEvent("contact.created").SetStatus(automation.StatusActive), "["+emailStep+"]")
	require.NoError(t, err)

	runIDs, err := jobs.EvaluateTrigger(ctx, env.DB.Scoped(fixtures.AcmeID), c.ID, "contact.created")
	require.NoError(t, err)
	require.Len(t, runIDs, 1)

	fs := &fakeSender{}
	mod := outbound.New(env.Bus, env.Holds, resolvingTo(fs), tracking.New("secret", "https://app.test"))
	_, err = jobs.RunStep(ctx, env.DB, noFence, mod, runIDs[0])
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

	c, err := env.DB.Contact.Create().SetWorkspaceID(fixtures.AcmeID).SetEmail("x@test.dev").Save(ctx)
	require.NoError(t, err)
	_, err = testhelper.AutomationWithSteps(ctx, env.DB, env.DB.Automation.Create().SetWorkspaceID(fixtures.AcmeID).
		SetName("Draft").SetTriggerEvent("contact.created"), "[]") // status defaults to draft
	require.NoError(t, err)

	runIDs, err := jobs.EvaluateTrigger(ctx, env.DB.Scoped(fixtures.AcmeID), c.ID, "contact.created")
	require.NoError(t, err)
	assert.Empty(t, runIDs)
}

func TestAutomationWaitDefersNextStep(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	c, err := env.DB.Contact.Create().SetWorkspaceID(fixtures.AcmeID).SetEmail("w@test.dev").Save(ctx)
	require.NoError(t, err)
	a, err := testhelper.AutomationWithSteps(ctx, env.DB, env.DB.Automation.Create().SetWorkspaceID(fixtures.AcmeID).
		SetName("Delayed").SetTriggerEvent("contact.created").SetStatus(automation.StatusActive), `[{"type":"wait","seconds":3600},`+emailStep+`]`)
	require.NoError(t, err)
	_ = a

	runIDs, err := jobs.EvaluateTrigger(ctx, env.DB.Scoped(fixtures.AcmeID), c.ID, "contact.created")
	require.NoError(t, err)
	require.Len(t, runIDs, 1)

	// First step is a wait: it defers (ResumeAt set), no send yet.
	res, err := jobs.RunStep(ctx, env.DB, noFence, newMod(env, resolvingTo(&fakeSender{})), runIDs[0])
	require.NoError(t, err)
	assert.False(t, res.Done)
	require.NotNil(t, res.ResumeAt)
	assert.NotNil(t, env.DB.AutomationRun.GetX(ctx, runIDs[0]).ResumeAt)
}

// A hold on the source (here: a suspended Workspace) leaves the enrollment exactly
// where it was and asks to be run again later; the run is neither failed nor exited.
func TestAutomationHeldStepWaitsAndResumes(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	// Fixtures: contact 130 (clean) and the active "hold_demo" automation (id 103).
	runIDs, err := jobs.EvaluateTrigger(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.ContactHoldDemoID, "hold_demo")
	require.NoError(t, err)
	require.Len(t, runIDs, 1)

	fs := &fakeSender{}
	mod := newMod(env, resolvingTo(fs))
	env.SuspendWorkspace(ctx, fixtures.AcmeID, time.Now())

	res, err := jobs.RunStep(ctx, env.DB, noFence, mod, runIDs[0])
	require.NoError(t, err)
	assert.False(t, res.Done)
	require.NotNil(t, res.ResumeAt, "the worker reschedules the same step")
	run := env.DB.AutomationRun.GetX(ctx, runIDs[0])
	assert.Equal(t, automationrun.StatusActive, run.Status)
	assert.Zero(t, run.CurrentStep, "a hold does not advance the enrollment")
	assert.Empty(t, fs.sent)

	env.UnsuspendWorkspace(ctx, fixtures.AcmeID)
	drive(t, env, resolvingTo(fs), runIDs[0])
	assert.Len(t, fs.sent, 1, "after the unfreeze the step sends")
}

// An Enrollment exited while a step is in flight stays exited: the late step's own
// outcome (here a send that can never be composed) finishes quietly instead of
// overwriting the exit.
func TestAutomationExitedMidStepStaysExitedAndTheLateStepFinishesQuietly(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)

	c, err := env.DB.Contact.Create().SetWorkspaceID(fixtures.AcmeID).
		SetEmail("late@test.dev").SetFirstName("Late").Save(ctx)
	require.NoError(t, err)
	a, err := testhelper.AutomationWithSteps(ctx, env.DB, env.DB.Automation.Create().SetWorkspaceID(fixtures.AcmeID).
		SetName("Broken template").SetTriggerEvent("contact.created").SetStatus(automation.StatusActive), `[{"type":"email","subject":"{% if x %}open","body":"<p>Hi</p>"}]`)
	require.NoError(t, err)
	runIDs, err := jobs.EvaluateTrigger(ctx, s, c.ID, "contact.created")
	require.NoError(t, err)
	require.Len(t, runIDs, 1)

	// The Contact unsubscribes while the step is already running: the provider lookup
	// is inside the step, after the run was read.
	resolver := &SendersMock{EmailSenderFunc: func(ctx context.Context, sc *ent.Scoped) (messaging.EmailSender, error) {
		_, err := automations.Exit(ctx, sc, a.ID, c.ID)
		require.NoError(t, err)
		return &fakeSender{}, nil
	}}

	res, err := jobs.RunStep(ctx, env.DB, noFence, newMod(env, resolver), runIDs[0])
	require.NoError(t, err)
	assert.True(t, res.Done)
	assert.Equal(t, automationrun.StatusExited, env.DB.AutomationRun.GetX(ctx, runIDs[0]).Status)
}

// exitedRun enrolls the Contact into the Automation and exits the Enrollment at once, so the step
// that runs next finds the run already exited.
func exitedRun(t *testing.T, env *testhelper.TestEnv, automationID, contactID int64) int64 {
	t.Helper()
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	runID, enrolled, err := automations.Enroll(ctx, s, automationID, contactID)
	require.NoError(t, err)
	require.True(t, enrolled)
	require.NoError(t, automations.ExitRun(ctx, s, runID))
	return runID
}

func TestStepOnAnExitedRunSendsNothingAndStaysPut(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	fs := &fakeSender{}
	runID := exitedRun(t, env, fixtures.AutomationHoldDemoID, fixtures.ContactAliceID)

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
	before := env.DB.Contact.GetX(ctx, fixtures.ContactBobID).QueryTags().CountX(ctx)
	runID := exitedRun(t, env, fixtures.AutomationTagOnEngagementID, fixtures.ContactBobID)

	res, err := jobs.RunStep(ctx, env.DB, noFence, newMod(env, resolvingTo(&fakeSender{})), runID)

	require.NoError(t, err)
	assert.True(t, res.Done)
	assert.Equal(t, before, env.DB.Contact.GetX(ctx, fixtures.ContactBobID).QueryTags().CountX(ctx))
	assert.Equal(t, 0, env.DB.AutomationRun.GetX(ctx, runID).CurrentStep)
}

func TestWaitStepOnAnExitedRunDoesNotSetResumeAt(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	runID, _, err := automations.Enroll(ctx, s, fixtures.AutomationWelcomeSeriesID, fixtures.ContactCarolID)
	require.NoError(t, err)
	_, err = jobs.RunStep(ctx, env.DB, noFence, newMod(env, resolvingTo(&fakeSender{})), runID) // the email
	require.NoError(t, err)
	require.NoError(t, automations.ExitRun(ctx, s, runID))

	res, err := jobs.RunStep(ctx, env.DB, noFence, newMod(env, resolvingTo(&fakeSender{})), runID) // the wait

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

	before, err := env.DB.Contact.Create().SetWorkspaceID(fixtures.AcmeID).SetEmail("before@test.dev").SetFirstName("Bea").Save(ctx)
	require.NoError(t, err)
	after, err := env.DB.Contact.Create().SetWorkspaceID(fixtures.AcmeID).SetEmail("after@test.dev").SetFirstName("Abe").Save(ctx)
	require.NoError(t, err)
	a, err := testhelper.AutomationWithSteps(ctx, env.DB, env.DB.Automation.Create().SetWorkspaceID(fixtures.AcmeID).
		SetName("Edited live").SetTriggerEvent("edit.live").SetStatus(automation.StatusActive),
		`[{"type":"wait","seconds":60},{"type":"email","subject":"Old steps","body":"<p>old</p>"}]`)
	require.NoError(t, err)

	early, err := jobs.EvaluateTrigger(ctx, acme, before.ID, "edit.live")
	require.NoError(t, err)
	require.Len(t, early, 1)
	// The first step (the wait) runs; the Enrollment now stands on the email step.
	fs := &fakeSender{}
	_, err = jobs.RunStep(ctx, env.DB, noFence, newMod(env, resolvingTo(fs)), early[0])
	require.NoError(t, err)

	// The owner edits the steps: a wait is inserted ahead and the email is reworded. Were the
	// Enrollment to read the live steps, its index 1 would now be the new wait.
	_, err = mod.Update(ctx, acme, a.ID, automations.UpdateInput{Steps: &[]automations.Step{
		{Type: automations.StepWait, Seconds: 1},
		{Type: automations.StepWait, Seconds: 2},
		{Type: automations.StepEmail, Subject: "New steps", Body: "<p>new</p>"},
	}})
	require.NoError(t, err)

	late, err := jobs.EvaluateTrigger(ctx, acme, after.ID, "edit.live")
	require.NoError(t, err)
	require.Len(t, late, 1)

	drive(t, env, resolvingTo(fs), early[0])
	drive(t, env, resolvingTo(fs), late[0])

	require.Len(t, fs.sent, 2)
	assert.Equal(t, "Old steps", fs.sent[0].Subject, "the Enrollment begun before the edit finished on its own version")
	assert.Equal(t, "New steps", fs.sent[1].Subject, "the Enrollment begun after the edit used the new steps")
	assert.Equal(t, automationrun.StatusCompleted, env.DB.AutomationRun.GetX(ctx, early[0]).Status)
	assert.NotEqual(t, env.DB.AutomationRun.GetX(ctx, early[0]).VersionID, env.DB.AutomationRun.GetX(ctx, late[0]).VersionID)
}
