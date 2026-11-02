package jobs_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/tag"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The send steps of an Automation (ADR 0081): each step sends through the Integration it names, a
// skipped send moves the Enrollment on, and a hold stops only the Enrollments at its steps. The
// fixture Automation emails through the default, waits, emails through the Mailgun relay, then
// applies the tag "walked-past".

// stepFigures is the Automation's report of its send steps, read through /api.
func stepFigures(t *testing.T, env *testhelper.TestEnv, automationID int64) []externalapi.AutomationStepFigures {
	t.Helper()
	res, err := env.ExternalScoped(t, "automations:read").AutomationsReport(t.Context(), externalapi.AutomationsReportParams{ID: externalapi.EntityId(automationID)})
	require.NoError(t, err)
	report, ok := res.(*externalapi.AutomationReport)
	require.Truef(t, ok, "got %T", res)
	return report.Steps
}

// tagNames is the names of the Contact's Tags, in name order.
func tagNames(t *testing.T, env *testhelper.TestEnv, contactID int64) []string {
	t.Helper()
	names, err := env.DB.Tag.Query().
		Where(tag.HasContactsWith(contact.ID(contactID))).
		Order(ent.Asc(tag.FieldName)).
		Select(tag.FieldName).
		Strings(context.Background())
	require.NoError(t, err)
	return names
}

// A Contact who cannot be emailed walks past both email steps to the tag step and completes: the
// skips do not end the sequence, and the report counts one skip at each email step.
func TestASkippedEmailMovesTheEnrollmentOnToTheStepsAfterIt(t *testing.T) {
	cannotBeEmailed := map[string]int64{
		"no email address":        fixtures.ContactJobsNoEmailID,
		"suppressed":              fixtures.ContactComplainedID,
		"unsubscribed everything": fixtures.ContactUnsubscribedEverythingID,
	}
	for name, contactID := range cannotBeEmailed {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			runID := enrollVia(t, env, fixtures.AutomationJobsEmailsThenTagID, contactID)
			fs := &fakeSender{}

			env.RunQueuedSteps(t, stepperOf(env, resolvingTo(fs)))

			assert.Empty(t, fs.sent, "no email goes out")
			assert.Equal(t, automationrun.StatusCompleted, env.DB.AutomationRun.GetX(t.Context(), runID).Status, "the skips do not end the Enrollment")
			assert.Contains(t, tagNames(t, env, contactID), "walked-past", "the step after the emails ran")
			assert.Equal(t, []externalapi.AutomationStepFigures{{Step: 0, Skipped: 1}, {Step: 2, Skipped: 1}}, stepFigures(t, env, fixtures.AutomationJobsEmailsThenTagID))
		})
	}
}

// Each email step sends through the Integration it names: the first through the Workspace default,
// the second through the Mailgun relay.
func TestEachEmailStepSendsThroughItsOwnIntegration(t *testing.T) {
	env := testhelper.Setup(t)
	runID := enrollVia(t, env, fixtures.AutomationJobsEmailsThenTagID, fixtures.ContactAliceID)
	fs := &fakeSender{}
	var through []int64

	env.RunQueuedSteps(t, stepperOf(env, recordingSenders(fs, &through)))

	assert.Equal(t, []string{"First", "Second"}, subjectsOf(fs))
	assert.Equal(t, []int64{fixtures.IntegrationAcmeDefaultID, fixtures.IntegrationAcmeMailgunID}, through)
	assert.Equal(t, automationrun.StatusCompleted, env.DB.AutomationRun.GetX(t.Context(), runID).Status)
	assert.Equal(t, []externalapi.AutomationStepFigures{{Step: 0, Sent: 1}, {Step: 2, Sent: 1}}, stepFigures(t, env, fixtures.AutomationJobsEmailsThenTagID))
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
			require.NoError(t, env.DB.Scoped(fixtures.AcmeID).Integration().Discard(t.Context(), fixtures.IntegrationAcmeMailgunID))
		},
	}
	for name, makeUnusable := range unusable {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			ctx := t.Context()
			makeUnusable(env)
			runID := enrollVia(t, env, fixtures.AutomationJobsEmailsThenTagID, fixtures.ContactAliceID)
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
			assert.Equal(t, 2, run.CurrentStep, "the Enrollment waits at the held step")
			assert.Equal(t, []map[string]any{stepJob(runID, 2)}, queuedSteps(t, env), "the held step is asked again later")
			assert.NotContains(t, tagNames(t, env, fixtures.ContactAliceID), "walked-past")
		})
	}
}
