package automations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

// A version no Enrollment stands on is rewritten rather than superseded, so a draft edited many
// times is one version.
func TestEditingTheStepsOfAnUnenrolledAutomationRewritesItsVersion(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer())
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	a, err := m.Get(ctx, acme, fixtures.AutomationWinBackID)
	require.NoError(t, err, "the fixture draft has no Enrollment")

	for _, seconds := range []int{2, 3, 4} {
		updated, err := m.Update(ctx, acme, a.ID, automations.UpdateInput{Steps: &[]automations.Step{{Type: automations.StepWait, Seconds: seconds}}})
		require.NoError(t, err)
		steps, err := automations.CurrentSteps(updated)
		require.NoError(t, err)
		assert.Equal(t, []automations.Step{{Type: automations.StepWait, Seconds: seconds}}, steps)
	}
	assert.Equal(t, []int{1}, versionsOf(t, env, a.ID))
}

// Once an Enrollment stands on the latest version, an edit becomes a new version and the old one
// keeps its steps and its Enrollments; the next Enrollment stands on the new one.
func TestEditingTheStepsUnderAnEnrollmentCreatesANewVersion(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer())
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	a, err := m.Get(ctx, acme, fixtures.AutomationWinBackID)
	require.NoError(t, err)
	original, err := automations.CurrentSteps(a)
	require.NoError(t, err)
	first, enrolled, err := automations.Enroll(ctx, acme, a.ID, fixtures.ContactAliceID)
	require.NoError(t, err)
	require.True(t, enrolled)

	edited := []automations.Step{{Type: automations.StepWait, Seconds: 99}}
	updated, err := m.Update(ctx, acme, a.ID, automations.UpdateInput{Steps: &edited})
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2}, versionsOf(t, env, a.ID))
	steps, err := automations.CurrentSteps(updated)
	require.NoError(t, err)
	assert.Equal(t, edited, steps, "the Automation now has the new steps")

	// Saving the same steps again stores nothing; the Enrollment still stands on version 1.
	_, err = m.Update(ctx, acme, a.ID, automations.UpdateInput{Steps: &edited})
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2}, versionsOf(t, env, a.ID))

	second, enrolled, err := automations.Enroll(ctx, acme, a.ID, fixtures.ContactBobID)
	require.NoError(t, err)
	require.True(t, enrolled)
	v1 := env.DB.AutomationRun.GetX(ctx, first).VersionID
	v2 := env.DB.AutomationRun.GetX(ctx, second).VersionID
	assert.NotEqual(t, v1, v2)
	assert.Equal(t, 1, env.DB.AutomationVersion.GetX(ctx, v1).Version)
	assert.Equal(t, 2, env.DB.AutomationVersion.GetX(ctx, v2).Version)
	old, err := automations.Decode(env.DB.AutomationVersion.GetX(ctx, v1).Steps)
	require.NoError(t, err)
	assert.Equal(t, original, old, "the version an Enrollment stands on is never rewritten")
}

// The fixture Automation whose steps were edited with Enrollments on the first version.
func TestFixtureAutomationHasEnrollmentsOnTwoVersions(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	assert.Equal(t, []int{1, 2}, versionsOf(t, env, fixtures.AutomationOnboardingDripID))
	assert.EqualValues(t, fixtures.AutomationOnboardingDripVersion1ID, env.DB.AutomationRun.GetX(ctx, fixtures.AutomationRunOnboardingOnVersion1ID).VersionID)
	assert.EqualValues(t, fixtures.AutomationOnboardingDripVersion2ID, env.DB.AutomationRun.GetX(ctx, fixtures.AutomationRunOnboardingOnVersion2ID).VersionID)
}
