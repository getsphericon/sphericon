package automations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
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

// stepsOf decodes the steps of a version the test expects to exist.
func stepsOf(t *testing.T, v *ent.AutomationVersion) []automations.Step {
	t.Helper()
	require.NotNil(t, v)
	steps, err := automations.Decode(v.Steps)
	require.NoError(t, err)
	return steps
}

// liveSteps and draftSteps are the steps of an Automation's live version and of its draft.
func liveSteps(t *testing.T, a *ent.Automation) []automations.Step {
	t.Helper()
	live, _ := automations.LiveAndDraft(a)
	return stepsOf(t, live)
}

func draftSteps(t *testing.T, a *ent.Automation) []automations.Step {
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

// A never-published Automation's draft is its whole definition: editing it rewrites the draft,
// so a sequence edited many times is one version, and nothing is live.
func TestEditingANeverPublishedAutomationRewritesItsDraft(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer())
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)

	_, err := m.Update(ctx, acme, fixtures.AutomationWinBackID, automations.UpdateInput{Steps: &[]automations.Step{{Type: automations.StepWait, Seconds: 2}}})
	require.NoError(t, err)
	updated, err := m.Update(ctx, acme, fixtures.AutomationWinBackID, automations.UpdateInput{Steps: &[]automations.Step{{Type: automations.StepWait, Seconds: 3}}})
	require.NoError(t, err)

	assert.Equal(t, []automations.Step{{Type: automations.StepWait, Seconds: 3}}, draftSteps(t, updated))
	live, _ := automations.LiveAndDraft(updated)
	assert.Nil(t, live, "nothing is live until it is published")
	assert.Equal(t, []int{1}, versionsOf(t, env, fixtures.AutomationWinBackID))
}

// Editing a live Automation writes a draft above the live version and never rewrites the live
// one: the next Enrollment still begins on what was published (#748). Later edits rewrite the
// draft in place.
func TestEditingALiveAutomationWritesADraftAndEnrollsOnTheLiveVersion(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer())
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	before, err := m.Get(ctx, acme, fixtures.AutomationHoldDemoID)
	require.NoError(t, err)
	published := liveSteps(t, before)

	_, err = m.Update(ctx, acme, fixtures.AutomationHoldDemoID, automations.UpdateInput{Steps: &[]automations.Step{{Type: automations.StepWait, Seconds: 60}}, TriggerEvent: new("hold_demo_edited")})
	require.NoError(t, err)
	edited := []automations.Step{{Type: automations.StepWait, Seconds: 120}}
	updated, err := m.Update(ctx, acme, fixtures.AutomationHoldDemoID, automations.UpdateInput{Steps: &edited})
	require.NoError(t, err)

	assert.Equal(t, edited, draftSteps(t, updated))
	assert.Equal(t, published, liveSteps(t, updated), "the live version is never rewritten")
	live, draft := automations.LiveAndDraft(updated)
	assert.Equal(t, "hold_demo", live.TriggerEvent)
	assert.Equal(t, "hold_demo_edited", draft.TriggerEvent, "the second edit keeps the draft's Trigger")
	assert.Equal(t, []int{1, 2}, versionsOf(t, env, fixtures.AutomationHoldDemoID), "one draft, rewritten in place")

	runID, enrolled, err := automations.Enroll(ctx, acme, fixtures.AutomationHoldDemoID, fixtures.ContactAliceID)
	require.NoError(t, err)
	require.True(t, enrolled)
	assert.Equal(t, 1, versionOfRun(t, env, runID), "the Enrollment begins on the live version")
}

// Saving the live definition again creates no draft; saving the draft's again changes nothing.
func TestSavingAnUnchangedDefinitionStoresNothing(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer())
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	before, err := m.Get(ctx, acme, fixtures.AutomationHoldDemoID)
	require.NoError(t, err)
	same := liveSteps(t, before)

	updated, err := m.Update(ctx, acme, fixtures.AutomationHoldDemoID, automations.UpdateInput{Steps: &same, TriggerEvent: new("hold_demo")})
	require.NoError(t, err)
	_, draft := automations.LiveAndDraft(updated)
	assert.Nil(t, draft)
	assert.Equal(t, []int{1}, versionsOf(t, env, fixtures.AutomationHoldDemoID))
}

// Publish makes the draft live: the next Enrollment follows the new version, the one in flight
// stays on the version it began on, and the old live version stays as history.
func TestPublishMakesTheDraftLiveAndInFlightEnrollmentsKeepTheirVersion(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer())
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	first, _, err := automations.Enroll(ctx, acme, fixtures.AutomationHoldDemoID, fixtures.ContactAliceID)
	require.NoError(t, err)
	edited := []automations.Step{{Type: automations.StepWait, Seconds: 99}}
	_, err = m.Update(ctx, acme, fixtures.AutomationHoldDemoID, automations.UpdateInput{Steps: &edited})
	require.NoError(t, err)

	published, err := m.Publish(ctx, acme, fixtures.AutomationHoldDemoID)
	require.NoError(t, err)
	assert.Equal(t, automation.StatusActive, published.Status)
	assert.Equal(t, edited, liveSteps(t, published))
	_, draft := automations.LiveAndDraft(published)
	assert.Nil(t, draft, "the draft became live")

	second, _, err := automations.Enroll(ctx, acme, fixtures.AutomationHoldDemoID, fixtures.ContactBobID)
	require.NoError(t, err)
	assert.Equal(t, 2, versionOfRun(t, env, second), "the next Enrollment follows the new version")
	assert.Equal(t, 1, versionOfRun(t, env, first), "the Enrollment in flight stays on its version")
	assert.Equal(t, []int{1, 2}, versionsOf(t, env, fixtures.AutomationHoldDemoID))
}

// With no draft, Publish resumes the live version of a deactivated Automation as it was.
func TestPublishWithoutADraftResumesTheLiveVersion(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer())
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	before, err := m.Deactivate(ctx, acme, fixtures.AutomationHoldDemoID)
	require.NoError(t, err)

	resumed, err := m.Publish(ctx, acme, fixtures.AutomationHoldDemoID)
	require.NoError(t, err)
	assert.Equal(t, automation.StatusActive, resumed.Status)
	assert.Equal(t, liveSteps(t, before), liveSteps(t, resumed))
	assert.Equal(t, []int{1}, versionsOf(t, env, fixtures.AutomationHoldDemoID))
}

// A never-published Automation enrolls nobody: it has no live version.
func TestANeverPublishedAutomationEnrollsNobody(t *testing.T) {
	env := testhelper.Setup(t)
	_, _, err := automations.Enroll(context.Background(), env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationWinBackID, fixtures.ContactAliceID)
	require.ErrorIs(t, err, automations.ErrNoVersion)
}

// Publish refuses a draft whose Integration was archived after it was saved, and changes nothing.
func TestPublishRefusesADraftWhoseIntegrationIsArchived(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer())
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	_, err := m.Update(ctx, acme, fixtures.AutomationHoldDemoID, automations.UpdateInput{IntegrationID: new(int64(fixtures.IntegrationAcmeMailgunID))})
	require.NoError(t, err)
	require.NoError(t, acme.Integration().Discard(ctx, fixtures.IntegrationAcmeMailgunID))

	_, err = m.Publish(ctx, acme, fixtures.AutomationHoldDemoID)
	require.ErrorIs(t, err, automations.ErrIntegrationNotFound)
	after, err := m.Get(ctx, acme, fixtures.AutomationHoldDemoID)
	require.NoError(t, err)
	live, draft := automations.LiveAndDraft(after)
	assert.Nil(t, live.IntegrationID, "the live version still sends through the default")
	assert.NotNil(t, draft, "the draft stays")
}

// Revert drops the draft above a live version; the live version is again the latest.
func TestRevertDropsTheDraft(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer())
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	before, err := m.Get(ctx, acme, fixtures.AutomationWelcomeSeriesID)
	require.NoError(t, err)

	reverted, err := m.Revert(ctx, acme, fixtures.AutomationWelcomeSeriesID)
	require.NoError(t, err)
	_, draft := automations.LiveAndDraft(reverted)
	assert.Nil(t, draft)
	assert.Equal(t, liveSteps(t, before), liveSteps(t, reverted))
	assert.Equal(t, []int{1}, versionsOf(t, env, fixtures.AutomationWelcomeSeriesID))

	_, err = m.Revert(ctx, acme, fixtures.AutomationWelcomeSeriesID)
	require.ErrorIs(t, err, automations.ErrNothingToRevert, "there is no draft any more")
}

// A never-published Automation's draft is its whole definition, so it is not reverted.
func TestRevertRefusesANeverPublishedAutomation(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer())
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)

	_, err := m.Revert(ctx, acme, fixtures.AutomationWinBackID)
	require.ErrorIs(t, err, automations.ErrNothingToRevert)
	after, err := m.Get(ctx, acme, fixtures.AutomationWinBackID)
	require.NoError(t, err)
	assert.NotEmpty(t, draftSteps(t, after), "the draft stays")
}

// The fixture Automation whose steps were edited with Enrollments on the first version.
func TestFixtureAutomationHasEnrollmentsOnTwoVersions(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	assert.Equal(t, []int{1, 2}, versionsOf(t, env, fixtures.AutomationOnboardingDripID))
	assert.EqualValues(t, fixtures.AutomationOnboardingDripVersion1ID, env.DB.AutomationRun.GetX(ctx, fixtures.AutomationRunOnboardingOnVersion1ID).VersionID)
	assert.EqualValues(t, fixtures.AutomationOnboardingDripVersion2ID, env.DB.AutomationRun.GetX(ctx, fixtures.AutomationRunOnboardingOnVersion2ID).VersionID)
}
