package jobs_test

import (
	"context"
	"testing"
	"time"

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

const emailStep = `{"type":"email","subject":"Hi {{ first_name }}","body":"<mjml><mj-body><mj-section><mj-column><mj-text>Hi {{ first_name }}</mj-text></mj-column></mj-section></mj-body></mjml>"}`

// drive runs RunStep until the run finishes (or a safety cap). tracker may be nil
// (most tests don't assert on the unsubscribe footer).
func drive(t *testing.T, env *testhelper.TestEnv, resolver outbound.Senders, runID int64) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		res, err := jobs.RunStep(ctx, env.DB, newMod(env, resolver), runID)
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

	_, err = env.DB.Automation.Create().SetWorkspaceID(fixtures.AcmeID).
		SetName("Welcome").SetTriggerEvent("contact.created").SetStatus(automation.StatusActive).
		SetDefinition("[" + emailStep + `,{"type":"wait","seconds":3600},` + emailStep + "]").
		Save(ctx)
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

	a, err := env.DB.Automation.Create().SetWorkspaceID(fixtures.AcmeID).
		SetName("Welcome").SetTriggerEvent("contact.created").SetStatus(automation.StatusActive).
		SetDefinition("[" + emailStep + "]").Save(ctx)
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

	a, err := env.DB.Automation.Create().SetWorkspaceID(fixtures.AcmeID).
		SetName("Welcome").SetTriggerEvent("contact.created").SetStatus(automation.StatusActive).
		SetDefinition("[" + emailStep + "]").Save(ctx)
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

	_, err = env.DB.Automation.Create().SetWorkspaceID(fixtures.AcmeID).
		SetName("Welcome").SetTriggerEvent("contact.created").SetStatus(automation.StatusActive).
		SetDefinition("[" + emailStep + "]").Save(ctx)
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
	_, err = env.DB.Automation.Create().SetWorkspaceID(fixtures.AcmeID).
		SetName("Welcome").SetTriggerEvent("contact.created").SetStatus(automation.StatusActive).
		SetDefinition("[" + emailStep + "]").Save(ctx)
	require.NoError(t, err)

	runIDs, err := jobs.EvaluateTrigger(ctx, env.DB.Scoped(fixtures.AcmeID), c.ID, "contact.created")
	require.NoError(t, err)
	require.Len(t, runIDs, 1)

	fs := &fakeSender{}
	mod := outbound.New(env.Bus, resolvingTo(fs), tracking.New("secret", "https://app.test"))
	_, err = jobs.RunStep(ctx, env.DB, mod, runIDs[0])
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
	_, err = env.DB.Automation.Create().SetWorkspaceID(fixtures.AcmeID).
		SetName("Draft").SetTriggerEvent("contact.created").Save(ctx) // status defaults to draft
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
	a, err := env.DB.Automation.Create().SetWorkspaceID(fixtures.AcmeID).
		SetName("Delayed").SetTriggerEvent("contact.created").SetStatus(automation.StatusActive).
		SetDefinition(`[{"type":"wait","seconds":3600},` + emailStep + `]`).Save(ctx)
	require.NoError(t, err)
	_ = a

	runIDs, err := jobs.EvaluateTrigger(ctx, env.DB.Scoped(fixtures.AcmeID), c.ID, "contact.created")
	require.NoError(t, err)
	require.Len(t, runIDs, 1)

	// First step is a wait: it defers (ResumeAt set), no send yet.
	res, err := jobs.RunStep(ctx, env.DB, newMod(env, resolvingTo(&fakeSender{})), runIDs[0])
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
	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSuspendedAt(time.Now()).ExecX(ctx)

	res, err := jobs.RunStep(ctx, env.DB, mod, runIDs[0])
	require.NoError(t, err)
	assert.False(t, res.Done)
	require.NotNil(t, res.ResumeAt, "the worker reschedules the same step")
	run := env.DB.AutomationRun.GetX(ctx, runIDs[0])
	assert.Equal(t, automationrun.StatusActive, run.Status)
	assert.Zero(t, run.CurrentStep, "a hold does not advance the enrollment")
	assert.Empty(t, fs.sent)

	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).ClearSuspendedAt().ExecX(ctx)
	drive(t, env, resolvingTo(fs), runIDs[0])
	assert.Len(t, fs.sent, 1, "after the unfreeze the step sends")
}
