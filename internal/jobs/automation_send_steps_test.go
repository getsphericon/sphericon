package jobs_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationsteprecord"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/tag"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/skipreason"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The send steps of an Automation (ADR 0081): each step sends through the Integration it names, a
// skipped send moves the Enrollment on, and a hold stops only the Enrollments at its steps. The
// fixture Automation emails through the default, waits, emails through the Mailgun relay, then
// applies the tag "walked-past".

// automationReport is the Automation's report, read through /api.
func automationReport(t *testing.T, env *testhelper.TestEnv, automationID int64) *externalapi.AutomationReport {
	t.Helper()
	report, err := env.ExternalScoped(t, apitokens.ScopeAutomationsRead).AutomationsReport(t.Context(), externalapi.AutomationsReportParams{ID: externalapi.EntityId(automationID)})
	require.NoError(t, err)
	return report
}

// stepFigures is the Automation's report of its send steps by Step id, read through /api.
func stepFigures(t *testing.T, env *testhelper.TestEnv, automationID int64) []externalapi.AutomationStepFigures {
	t.Helper()
	return automationReport(t, env, automationID).Steps
}

// The ids of the fixture Automation "Jobs emails then tag"'s two email steps, at positions 0 and 2.
const (
	firstEmailStepID  = "11bbd2a8-1c74-5ea8-8d63-b2bb27f4f0c6"
	secondEmailStepID = "5c33d45c-1063-576c-b4d6-673ee91ac7f2"
)

// tagNames is the names of the Contact's Tags, in name order.
func tagNames(t *testing.T, env *testhelper.TestEnv, contactID int64) []string {
	t.Helper()
	names, err := env.DB.Tag.Query().
		Where(tag.HasContactsWith(contact.ID(contactID))).
		Order(ent.Asc(tag.FieldName)).
		Select(tag.FieldName).
		Strings(t.Context())
	require.NoError(t, err)
	return names
}

// A Contact who cannot be emailed walks past both email steps to the tag step and completes: the
// skips do not end the sequence, and the report counts one skip at each email step.
func TestASkippedEmailMovesTheEnrollmentOnToTheStepsAfterIt(t *testing.T) {
	cannotBeEmailed := map[string]struct{ runID, contactID int64 }{
		"no email address":        {fixtures.AutomationRunJobsEmailsThenTagNoEmailID, fixtures.ContactJobsNoEmailID},
		"suppressed":              {fixtures.AutomationRunJobsEmailsThenTagComplainedID, fixtures.ContactComplainedID},
		"unsubscribed everything": {fixtures.AutomationRunJobsEmailsThenTagUnsubscribedEverythingID, fixtures.ContactUnsubscribedEverythingID},
	}
	for name, enrolled := range cannotBeEmailed {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			runID, contactID := enrolled.runID, enrolled.contactID
			queueStep(t, env, runID)
			fs := &fakeSender{}

			env.RunQueuedSteps(t, stepperOf(env, resolvingTo(fs)))

			assert.Empty(t, fs.sent, "no email goes out")
			assert.Equal(t, automationrun.StatusCompleted, env.DB.AutomationRun.GetX(t.Context(), runID).Status, "the skips do not end the Enrollment")
			assert.Contains(t, tagNames(t, env, contactID), "walked-past", "the step after the emails ran")
			assert.Equal(t, []externalapi.AutomationStepFigures{{StepId: firstEmailStepID, Skipped: 1}, {StepId: secondEmailStepID, Skipped: 1}}, stepFigures(t, env, fixtures.AutomationJobsEmailsThenTagID))
		})
	}
}

// A send Step the Enrollment moved on from without sending records that it was skipped, and why
// (ADR 0124); the Steps it did run record that they were done.
func TestASkippedEmailStepRecordsWhy(t *testing.T) {
	cannotBeEmailed := map[string]struct {
		runID int64
		why   skipreason.Reason
	}{
		"no email address":        {fixtures.AutomationRunJobsEmailsThenTagNoEmailID, skipreason.NoDestination},
		"suppressed":              {fixtures.AutomationRunJobsEmailsThenTagComplainedID, skipreason.Suppressed},
		"unsubscribed everything": {fixtures.AutomationRunJobsEmailsThenTagUnsubscribedEverythingID, skipreason.UnsubscribedEverything},
	}
	for name, enrolled := range cannotBeEmailed {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			queueStep(t, env, enrolled.runID)

			env.RunQueuedSteps(t, stepperOf(env, resolvingTo(&fakeSender{})))

			records := stepRecordsOf(t, env, enrolled.runID)
			assert.Equal(t, []automationsteprecord.Outcome{automationsteprecord.OutcomeSkipped, automationsteprecord.OutcomeDone, automationsteprecord.OutcomeSkipped, automationsteprecord.OutcomeDone},
				lo.Map(records, func(r *ent.AutomationStepRecord, _ int) automationsteprecord.Outcome { return lo.FromPtr(r.Outcome) }))
			assert.Equal(t, []skipreason.Reason{enrolled.why, "", enrolled.why, ""},
				lo.Map(records, func(r *ent.AutomationStepRecord, _ int) skipreason.Reason { return lo.FromPtr(r.SkipReason) }))
		})
	}
}

// Each email step sends through the Integration it names: the first through the Workspace default,
// the second through the Mailgun relay.
func TestEachEmailStepSendsThroughItsOwnIntegration(t *testing.T) {
	env := testhelper.Setup(t)
	runID := int64(fixtures.AutomationRunJobsEmailsThenTagAliceID)
	queueStep(t, env, runID)
	fs := &fakeSender{}
	var through []int64

	env.RunQueuedSteps(t, stepperOf(env, recordingSenders(fs, &through)))

	assert.Equal(t, []string{"First", "Second"}, subjectsOf(fs))
	assert.Equal(t, []int64{fixtures.IntegrationAcmeDefaultID, fixtures.IntegrationAcmeMailgunID}, through)
	assert.Equal(t, automationrun.StatusCompleted, env.DB.AutomationRun.GetX(t.Context(), runID).Status)
	assert.Equal(t, []externalapi.AutomationStepFigures{{StepId: firstEmailStepID, Sent: 1}, {StepId: secondEmailStepID, Sent: 1}}, stepFigures(t, env, fixtures.AutomationJobsEmailsThenTagID))
}

// A held Integration stops only the Enrollments at the steps that send through it: the step
// through the default sends, and the Enrollment then waits at the step through the relay that
// cannot send, asking again later, neither moved on nor ended, and nothing falls back to the
// default.
func TestAHeldIntegrationStopsOnlyTheStepsThatSendThroughIt(t *testing.T) {
	unusable := map[string]func(env *testhelper.TestEnv){
		"disabled": func(env *testhelper.TestEnv) {
			env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeMailgunID).SetEnabled(false).ExecX(t.Context())
		},
		"archived": func(env *testhelper.TestEnv) {
			require.NoError(t, env.DB.Scoped(fixtures.AcmeID).Integration().Discard(t.Context(), fixtures.IntegrationAcmeMailgunID, time.Now()))
		},
	}
	for name, makeUnusable := range unusable {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			ctx := t.Context()
			makeUnusable(env)
			runID := int64(fixtures.AutomationRunJobsEmailsThenTagAliceID)
			queueStep(t, env, runID)
			fs := &fakeSender{}
			var through []int64
			mod := stepperOf(env, recordingSenders(fs, &through))

			require.True(t, env.RunNextStep(t, mod)) // the email through the default
			require.True(t, env.RunNextStep(t, mod)) // the wait
			require.True(t, env.RunNextStep(t, mod)) // the email through the relay

			assert.Equal(t, []string{"First"}, subjectsOf(fs), "the default's step sent")
			assert.Equal(t, []int64{fixtures.IntegrationAcmeDefaultID}, through, "nothing fell back to the default")
			run := env.DB.AutomationRun.GetX(ctx, runID)
			assert.Equal(t, automationrun.StatusActive, run.Status)
			assert.Equal(t, env.StepIDAt(t, runID, 2), lo.FromPtr(run.CurrentStepID), "the Enrollment waits at the held step")
			assert.Equal(t, []map[string]any{stepJob(t, env, runID, 2)}, queuedSteps(t, env), "the held step is asked again later")
			assert.NotContains(t, tagNames(t, env, fixtures.ContactAliceID), "walked-past")
		})
	}
}

// editableEmailStepID is the id of the email step of the fixture Automation "Jobs editable", which
// its version 1 holds after a wait.
const editableEmailStepID = "e8ca8a35-46c4-53e4-bc98-00181ff0a38d"

// A send Step keeps its figures and its messages' keys when a new version inserts a Step above it
// (ADR 0122): the report sums what both versions' Enrollments sent at the Step by its id, and
// shows each version's share, and each message's key names the Step by its id, not its place.
func TestASendStepKeepsItsFiguresAndKeysWhenAStepIsInsertedAboveIt(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	mod := testhelper.Module[*automations.Module](env)
	stepper := stepperOf(env, resolvingTo(&fakeSender{}))
	early := enrollVia(t, env, fixtures.AutomationJobsEditableID, fixtures.ContactAliceID)
	env.RunQueuedSteps(t, stepper)
	// Version 2 inserts a wait above the email, which keeps its id.
	_, err := mod.Update(ctx, acme, fixtures.AutomationJobsEditableID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, acme, fixtures.AutomationJobsEditableID), Steps: new(automationstep.Root(
		automationstep.New(automationstep.Wait{Seconds: 60}),
		automationstep.New(automationstep.Wait{Seconds: 1}),
		automationstep.Step{ID: editableEmailStepID, Payload: automationstep.Email{Subject: "Old steps", Body: "<p>old</p>"}},
	))})
	require.NoError(t, err)
	_, err = mod.Publish(ctx, acme, fixtures.AutomationJobsEditableID, testhelper.AutomationRevisionIn(ctx, t, acme, fixtures.AutomationJobsEditableID))
	require.NoError(t, err)
	late := enrollVia(t, env, fixtures.AutomationJobsEditableID, fixtures.ContactCarolID)

	env.RunQueuedSteps(t, stepper)

	report := automationReport(t, env, fixtures.AutomationJobsEditableID)
	assert.Equal(t, []externalapi.AutomationStepFigures{{StepId: editableEmailStepID, Sent: 2}}, report.Steps)
	assert.Equal(t, []externalapi.AutomationVersionStepFigures{
		{Version: 1, StepId: editableEmailStepID, Sent: 1},
		{Version: 2, StepId: editableEmailStepID, Sent: 1},
	}, report.VersionSteps)
	keys := env.DB.OutboundMessage.Query().
		Where(outboundmessage.AutomationRunIDIn(early, late)).
		Order(ent.Asc(outboundmessage.FieldID)).
		Select(outboundmessage.FieldIdempotencyKey).
		StringsX(ctx)
	assert.Equal(t, []string{
		fmt.Sprintf("automation:%d:%s", early, editableEmailStepID),
		fmt.Sprintf("automation:%d:%s", late, editableEmailStepID),
	}, keys)
}
