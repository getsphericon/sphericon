package automations_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func versionsOf(t *testing.T, env *testhelper.TestEnv, automationID int64) []int {
	t.Helper()
	rows, err := env.DB.AutomationVersion.Query().
		Where(automationversion.AutomationID(automationID)).
		Order(automationversion.ByVersion()).All(context.Background())
	require.NoError(t, err)
	out := make([]int, len(rows))
	for i, r := range rows {
		out[i] = r.Version
	}
	return out
}

// pathOf is the steps of a version the test expects to exist.
func pathOf(t *testing.T, v *ent.AutomationVersion) automationstep.Path {
	t.Helper()
	require.NotNil(t, v)
	return v.Steps
}

// stepsOf is what each step of a version the test expects to exist does, in order, without the
// ids the save gave them.
func stepsOf(t *testing.T, v *ent.AutomationVersion) []automationstep.Payload {
	t.Helper()
	return payloadsOf(pathOf(t, v))
}

// payloadsOf is what each step of the path does, in order.
func payloadsOf(p automationstep.Path) []automationstep.Payload {
	out := []automationstep.Payload{}
	for _, st := range automationstep.All(p) {
		out = append(out, st.Payload)
	}
	return out
}

// steps is a root path of the payloads given, for a save that gives the path and its steps their
// ids.
func steps(payloads ...automationstep.Payload) *automationstep.Path {
	p := automationstep.Root()
	for _, payload := range payloads {
		p.Steps = append(p.Steps, automationstep.New(payload))
	}
	return &p
}

// liveSteps and draftSteps are what the steps of an Automation's live version and of its draft do.
func liveSteps(t *testing.T, a *ent.Automation) []automationstep.Payload {
	t.Helper()
	live, _ := automations.LiveAndDraft(a)
	return stepsOf(t, live)
}

func draftSteps(t *testing.T, a *ent.Automation) []automationstep.Payload {
	t.Helper()
	_, draft := automations.LiveAndDraft(a)
	return stepsOf(t, draft)
}

// versionOfRun is the number of the version an Enrollment stands on.
func versionOfRun(t *testing.T, env *testhelper.TestEnv, runID int64) int {
	t.Helper()
	ctx := context.Background()
	run, err := env.DB.AutomationRun.Get(ctx, runID)
	require.NoError(t, err)
	v, err := env.DB.AutomationVersion.Get(ctx, run.VersionID)
	require.NoError(t, err)
	return v.Version
}

// The lifecycle the Automation shares with the Rule (edit, Publish, Revert, Archive) is tested once,
// for both, in internal/domain/content/versioneddefinition. These are what only an Automation has: Enrollments
// on a version, steps, and the Integration.

// Editing a live Automation writes a draft above the live version and never rewrites the live
// one: the next Enrollment still begins on what was published (#748). Later edits rewrite the
// draft in place.
func TestEditingALiveAutomationWritesADraftAndEnrollsOnTheLiveVersion(t *testing.T) {
	env := testhelper.Setup(t)
	m := testhelper.Module[*automations.Module](env)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	before, err := m.Get(ctx, acme, fixtures.AutomationHoldDemoID)
	require.NoError(t, err)
	published := liveSteps(t, before)

	_, err = m.Update(ctx, acme, fixtures.AutomationHoldDemoID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, acme, fixtures.AutomationHoldDemoID), Steps: steps(automationstep.Wait{Seconds: 60}), Trigger: &eventmatcher.Matcher{Action: "hold_demo_edited"}})
	require.NoError(t, err)
	edited := []automationstep.Payload{automationstep.Wait{Seconds: 120}}
	updated, err := m.Update(ctx, acme, fixtures.AutomationHoldDemoID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, acme, fixtures.AutomationHoldDemoID), Steps: steps(edited...)})
	require.NoError(t, err)

	assert.Equal(t, edited, draftSteps(t, updated))
	assert.Equal(t, published, liveSteps(t, updated), "the live version is never rewritten")
	live, draft := automations.LiveAndDraft(updated)
	assert.Equal(t, "hold_demo", live.TriggerAction)
	assert.Equal(t, "hold_demo_edited", draft.TriggerAction, "the second edit keeps the draft's Trigger")
	assert.Equal(t, []int{1, 2}, versionsOf(t, env, fixtures.AutomationHoldDemoID), "one draft, rewritten in place")

	enrolled, err := enrollNow(t, env, acme, fixtures.AutomationHoldDemoID, fixtures.ContactAliceID)
	require.NoError(t, err)
	require.True(t, enrolled)
	runID := runOf(t, env, fixtures.AutomationHoldDemoID, fixtures.ContactAliceID)
	assert.Equal(t, 1, versionOfRun(t, env, runID), "the Enrollment begins on the live version")
}

// Saving the live steps again, with their ids, creates no draft: steps compare in the form a
// version stores them.
func TestSavingUnchangedStepsStoresNothing(t *testing.T) {
	env := testhelper.Setup(t)
	m := testhelper.Module[*automations.Module](env)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	before, err := m.Get(ctx, acme, fixtures.AutomationHoldDemoID)
	require.NoError(t, err)
	live, _ := automations.LiveAndDraft(before)
	same := pathOf(t, live)

	updated, err := m.Update(ctx, acme, fixtures.AutomationHoldDemoID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, acme, fixtures.AutomationHoldDemoID), Steps: &same, Trigger: &eventmatcher.Matcher{Action: "hold_demo"}})
	require.NoError(t, err)
	_, draft := automations.LiveAndDraft(updated)
	assert.Nil(t, draft)
	assert.Equal(t, []int{1}, versionsOf(t, env, fixtures.AutomationHoldDemoID))
}

// Publish makes the draft live: the next Enrollment follows the new version, the one in flight
// stays on the version it began on, and the old live version stays as history.
func TestPublishMakesTheDraftLiveAndInFlightEnrollmentsKeepTheirVersion(t *testing.T) {
	env := testhelper.Setup(t)
	m := testhelper.Module[*automations.Module](env)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	_, err := enrollNow(t, env, acme, fixtures.AutomationHoldDemoID, fixtures.ContactAliceID)
	require.NoError(t, err)
	first := runOf(t, env, fixtures.AutomationHoldDemoID, fixtures.ContactAliceID)
	edited := []automationstep.Payload{automationstep.Wait{Seconds: 99}}
	_, err = m.Update(ctx, acme, fixtures.AutomationHoldDemoID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, acme, fixtures.AutomationHoldDemoID), Steps: steps(edited...)})
	require.NoError(t, err)

	published, err := m.Publish(ctx, acme, fixtures.AutomationHoldDemoID, testhelper.AutomationRevisionIn(ctx, t, acme, fixtures.AutomationHoldDemoID))
	require.NoError(t, err)
	assert.Equal(t, automation.StatusActive, published.Status)
	assert.Equal(t, edited, liveSteps(t, published))
	_, draft := automations.LiveAndDraft(published)
	assert.Nil(t, draft, "the draft became live")

	_, err = enrollNow(t, env, acme, fixtures.AutomationHoldDemoID, fixtures.ContactBobID)
	require.NoError(t, err)
	second := runOf(t, env, fixtures.AutomationHoldDemoID, fixtures.ContactBobID)
	assert.Equal(t, 2, versionOfRun(t, env, second), "the next Enrollment follows the new version")
	assert.Equal(t, 1, versionOfRun(t, env, first), "the Enrollment in flight stays on its version")
	assert.Equal(t, []int{1, 2}, versionsOf(t, env, fixtures.AutomationHoldDemoID))
}

// A never-published Automation enrolls nobody: it has no live version.
func TestANeverPublishedAutomationEnrollsNobody(t *testing.T) {
	env := testhelper.Setup(t)
	_, err := enrollNow(t, env, env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationWinBackID, fixtures.ContactAliceID)
	require.ErrorIs(t, err, automations.ErrNoVersion)
}

// Publish refuses a draft whose Integration was archived after it was saved, and changes nothing.
func TestPublishRefusesADraftWhoseIntegrationIsArchived(t *testing.T) {
	env := testhelper.Setup(t)
	m := testhelper.Module[*automations.Module](env)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	mailgun := int64(fixtures.IntegrationAcmeMailgunID)
	_, err := m.Update(ctx, acme, fixtures.AutomationHoldDemoID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, acme, fixtures.AutomationHoldDemoID), Steps: steps(emailVia(&mailgun))})
	require.NoError(t, err)
	require.NoError(t, acme.Integration().Discard(ctx, fixtures.IntegrationAcmeMailgunID, time.Now()))

	_, err = m.Publish(ctx, acme, fixtures.AutomationHoldDemoID, testhelper.AutomationRevisionIn(ctx, t, acme, fixtures.AutomationHoldDemoID))
	require.ErrorIs(t, err, automations.ErrIntegrationNotFound)
	after, err := m.Get(ctx, acme, fixtures.AutomationHoldDemoID)
	require.NoError(t, err)
	live, draft := automations.LiveAndDraft(after)
	assert.Equal(t, []*int64{nil}, stepIntegrations(live), "the live version still sends through the default")
	assert.NotNil(t, draft, "the draft stays")
}

// The fixture Automation whose steps were edited with Enrollments on the first version.
func TestFixtureAutomationHasEnrollmentsOnTwoVersions(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	assert.Equal(t, []int{1, 2}, versionsOf(t, env, fixtures.AutomationOnboardingDripID))
	assert.EqualValues(t, fixtures.AutomationOnboardingDripVersion1ID, env.DB.AutomationRun.GetX(ctx, fixtures.AutomationRunOnboardingOnVersion1ID).VersionID)
	assert.EqualValues(t, fixtures.AutomationOnboardingDripVersion2ID, env.DB.AutomationRun.GetX(ctx, fixtures.AutomationRunOnboardingOnVersion2ID).VersionID)
}
