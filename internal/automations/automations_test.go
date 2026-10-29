package automations_test

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestDecodeReadsTheStoredDefinition(t *testing.T) {
	steps, err := automations.Decode(`[{"type":"email","subject":"Hi","body":"<p></p>"},{"type":"wait","seconds":60},{"type":"apply_tag","tag":"vip"}]`)
	require.NoError(t, err)
	assert.Equal(t, []automations.Step{
		{Type: automations.StepEmail, Subject: "Hi", Body: "<p></p>"},
		{Type: automations.StepWait, Seconds: 60},
		{Type: automations.StepApplyTag, Tag: "vip"},
	}, steps)

	none, err := automations.Decode("")
	require.NoError(t, err)
	assert.Empty(t, none, "an empty definition has no steps")

	_, err = automations.Decode("{not json")
	require.Error(t, err)
}

func TestEncodeKeepsOnlyTheFieldsOfEachStepType(t *testing.T) {
	def, err := automations.Encode([]automations.Step{
		{Type: automations.StepEmail, Subject: "Hi", Body: "b", Seconds: 9, Tag: "stray"},
		{Type: automations.StepWait, Seconds: 30, Subject: "stray"},
		{Type: automations.StepApplyTag, Tag: "  vip ", Body: "stray"},
		{Type: automations.StepRemoveTag, Tag: "old"},
		{Subject: "untyped steps are emails"},
	})
	require.NoError(t, err)
	assert.JSONEq(t, `[
		{"type":"email","subject":"Hi","body":"b"},
		{"type":"wait","seconds":30},
		{"type":"apply_tag","tag":"vip"},
		{"type":"remove_tag","tag":"old"},
		{"type":"email","subject":"untyped steps are emails"}
	]`, def)

	empty, err := automations.Encode(nil)
	require.NoError(t, err)
	assert.Equal(t, "[]", empty)
}

func TestEncodeSanitizesAnEmailStepBody(t *testing.T) {
	def, err := automations.Encode([]automations.Step{
		{Type: automations.StepEmail, Subject: "Hi", Body: `<p onclick="x()">Hi {{ first_name }}</p><script>alert(1)</script>`},
	})
	require.NoError(t, err)
	assert.JSONEq(t, `[{"type":"email","subject":"Hi","body":"<p>Hi {{ first_name }}</p>"}]`, def)
}

func TestCreateStoresADraftWithEncodedSteps(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer())
	ctx := context.Background()

	a, err := m.Create(ctx, env.DB.Scoped(fixtures.AcmeID), automations.CreateInput{
		Name: "Onboarding", TriggerEvent: "signup",
		Steps: []automations.Step{{Type: automations.StepApplyTag, Tag: " new "}, {Type: automations.StepWait, Seconds: 5}},
	})
	require.NoError(t, err)
	assert.Equal(t, automation.StatusDraft, a.Status)
	assert.EqualValues(t, fixtures.AcmeID, a.WorkspaceID)
	steps, err := automations.CurrentSteps(a)
	require.NoError(t, err)
	assert.Equal(t, []automations.Step{{Type: automations.StepApplyTag, Tag: "new"}, {Type: automations.StepWait, Seconds: 5}}, steps)

	bare, err := m.Create(ctx, env.DB.Scoped(fixtures.AcmeID), automations.CreateInput{Name: "No steps", TriggerEvent: "x"})
	require.NoError(t, err)
	none, err := automations.CurrentSteps(bare)
	require.NoError(t, err)
	assert.Empty(t, none)
}

// automationNames are the names of Acme's Automations.
func automationNames(t *testing.T, env *testhelper.TestEnv) []string {
	t.Helper()
	names, err := env.DB.Scoped(fixtures.AcmeID).Automation().Query().Select(automation.FieldName).Strings(context.Background())
	require.NoError(t, err)
	return names
}

func TestInvalidStepsAreRefusedAndNothingIsStored(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer())
	ctx := context.Background()
	before := automationNames(t, env)

	for name, steps := range map[string][]automations.Step{
		"unknown type":        {{Type: "sms"}},
		"negative wait":       {{Type: automations.StepWait, Seconds: -1}},
		"apply_tag blank tag": {{Type: automations.StepApplyTag, Tag: "  "}},
		"remove_tag no tag":   {{Type: automations.StepRemoveTag}},
	} {
		_, err := m.Create(ctx, env.DB.Scoped(fixtures.AcmeID), automations.CreateInput{Name: "x", TriggerEvent: "y", Steps: steps})
		require.ErrorIs(t, err, automations.ErrInvalidStep, name)

		_, err = m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationWelcomeSeriesID, automations.UpdateInput{Steps: &steps})
		require.ErrorIs(t, err, automations.ErrInvalidStep, "update: "+name)
	}

	assert.ElementsMatch(t, before, automationNames(t, env), "nothing was stored")
	welcome, err := m.Get(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationWelcomeSeriesID)
	require.NoError(t, err)
	stored, err := automations.CurrentSteps(welcome)
	require.NoError(t, err)
	assert.Len(t, stored, 3, "a refused update leaves the steps as they were")
}

func TestListIsNewestFirstWithinTheWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	page, err := automations.New(env.Cipher, env.Bus, testhelper.Renderer()).List(context.Background(), env.DB.Scoped(fixtures.AcmeID), pagination.Params{Page: 1, PageSize: 100}, false)
	require.NoError(t, err)
	require.Greater(t, page.TotalItems, 1)
	for _, a := range page.Items {
		assert.EqualValues(t, fixtures.AcmeID, a.WorkspaceID, "another tenant's automation leaked")
	}
	ids := lo.Map(page.Items, func(a *ent.Automation, _ int) int64 { return a.ID })
	assert.IsDecreasing(t, ids, "newest first")
}

func TestUpdateChangesOnlyWhatIsGivenAndKeepsTheStatus(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer())
	ctx := context.Background()
	before, err := m.Get(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationWelcomeSeriesID)
	require.NoError(t, err)

	renamed, err := m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), before.ID, automations.UpdateInput{Name: new("Renamed"), TriggerEvent: new("new.trigger")})
	require.NoError(t, err)
	assert.Equal(t, "Renamed", renamed.Name)
	assert.Equal(t, "new.trigger", renamed.TriggerEvent)
	beforeSteps, err := automations.CurrentSteps(before)
	require.NoError(t, err)
	renamedSteps, err := automations.CurrentSteps(renamed)
	require.NoError(t, err)
	assert.Equal(t, beforeSteps, renamedSteps, "steps untouched when not given")
	assert.Equal(t, before.Status, renamed.Status)

	resteps, err := m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), before.ID, automations.UpdateInput{Steps: &[]automations.Step{{Type: automations.StepWait, Seconds: 1}}})
	require.NoError(t, err)
	steps, err := automations.CurrentSteps(resteps)
	require.NoError(t, err)
	assert.Equal(t, []automations.Step{{Type: automations.StepWait, Seconds: 1}}, steps)
}

func TestActivateAndDeactivateFlipTheStatus(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer())
	ctx := context.Background()

	off, err := m.Deactivate(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationWelcomeSeriesID)
	require.NoError(t, err)
	assert.Equal(t, automation.StatusDraft, off.Status)
	on, err := m.Activate(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationWelcomeSeriesID)
	require.NoError(t, err)
	assert.Equal(t, automation.StatusActive, on.Status)

	stored, err := env.DB.Automation.Get(ctx, fixtures.AutomationWelcomeSeriesID)
	require.NoError(t, err)
	assert.Equal(t, automation.StatusActive, stored.Status)
}

func TestDeleteRemovesTheAutomation(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer())
	ctx := context.Background()

	require.NoError(t, m.Delete(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationWinBackID))
	_, err := m.Get(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationWinBackID)
	require.ErrorIs(t, err, automations.ErrNotFound)
	assert.ErrorIs(t, m.Delete(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationWinBackID), automations.ErrNotFound, "deleting twice")
}

// Every operation is workspace-scoped: another tenant's id is "not found" and the
// row is untouched.
func TestOperationsNeverReachAnotherWorkspacesAutomation(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer())
	ctx := context.Background()
	foreign := env.DB.Automation.GetX(ctx, fixtures.AutomationGlobexID)

	_, err := m.Get(ctx, env.DB.Scoped(fixtures.AcmeID), foreign.ID)
	require.ErrorIs(t, err, automations.ErrNotFound)
	_, err = m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), foreign.ID, automations.UpdateInput{Name: new("hijacked")})
	require.ErrorIs(t, err, automations.ErrNotFound)
	_, err = m.Activate(ctx, env.DB.Scoped(fixtures.AcmeID), foreign.ID)
	require.ErrorIs(t, err, automations.ErrNotFound)
	_, err = m.Deactivate(ctx, env.DB.Scoped(fixtures.AcmeID), foreign.ID)
	require.ErrorIs(t, err, automations.ErrNotFound)
	require.ErrorIs(t, m.Delete(ctx, env.DB.Scoped(fixtures.AcmeID), foreign.ID), automations.ErrNotFound)

	_, err = m.Get(ctx, env.DB.Scoped(fixtures.AcmeID), 999999)
	require.ErrorIs(t, err, automations.ErrNotFound, "an unknown id")

	stored, err := env.DB.Automation.Get(ctx, foreign.ID)
	require.NoError(t, err)
	assert.Equal(t, fixtures.AutomationGlobexName, stored.Name)
	assert.Equal(t, automation.StatusDraft, stored.Status)
}

// An archived Automation enrolls nobody, so it is neither activated nor deactivated: both are
// "not found" until it is restored, and its status stays as it was.
func TestAnArchivedAutomationKeepsItsStatus(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer())
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	require.NoError(t, m.Archive(ctx, acme, fixtures.AutomationNotifySlackID))
	before, err := m.Get(ctx, acme, fixtures.AutomationNotifySlackID)
	require.NoError(t, err)

	_, err = m.Activate(ctx, acme, fixtures.AutomationNotifySlackID)
	require.ErrorIs(t, err, automations.ErrNotFound)
	_, err = m.Deactivate(ctx, acme, fixtures.AutomationNotifySlackID)
	require.ErrorIs(t, err, automations.ErrNotFound)

	after, err := m.Get(ctx, acme, fixtures.AutomationNotifySlackID)
	require.NoError(t, err)
	assert.Equal(t, before.Status, after.Status)
}

// UsingTag names the Automations whose steps apply or remove the Tag, archived ones too:
// restoring one must not leave it naming a missing Tag.
func TestUsingTagIncludesArchivedAutomations(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	require.NoError(t, acme.Automation().Discard(ctx, fixtures.AutomationTagOnEngagementID))

	using, err := automations.UsingTag(ctx, acme, fixtures.TagVipName)
	require.NoError(t, err)
	names := lo.Map(using, func(a *ent.Automation, _ int) string { return a.Name })
	assert.Equal(t, []string{fixtures.AutomationTagOnEngagementName}, names)
}

func TestAnAutomationSavesAndClearsTheIntegrationItSendsThrough(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer())
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)

	a, err := m.Create(ctx, acme, automations.CreateInput{
		Name: "Onboarding", TriggerEvent: "signup", IntegrationID: new(int64(fixtures.IntegrationAcmeMailgunID)),
	})
	require.NoError(t, err)
	require.NotNil(t, a.IntegrationID)
	assert.EqualValues(t, fixtures.IntegrationAcmeMailgunID, *a.IntegrationID)

	renamed, err := m.Update(ctx, acme, a.ID, automations.UpdateInput{Name: new("Renamed")})
	require.NoError(t, err)
	require.NotNil(t, renamed.IntegrationID, "an update that does not name an Integration keeps it")

	moved, err := m.Update(ctx, acme, a.ID, automations.UpdateInput{IntegrationID: new(int64(fixtures.IntegrationAcmeDefaultID))})
	require.NoError(t, err)
	require.NotNil(t, moved.IntegrationID)
	assert.EqualValues(t, fixtures.IntegrationAcmeDefaultID, *moved.IntegrationID)

	cleared, err := m.Update(ctx, acme, a.ID, automations.UpdateInput{ClearIntegration: true})
	require.NoError(t, err)
	assert.Nil(t, cleared.IntegrationID, "cleared, the Automation sends through the default")
}

func TestAnAutomationRefusesAnIntegrationThatIsMissingOrArchived(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer())
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	require.NoError(t, acme.Integration().Discard(ctx, fixtures.IntegrationAcmeMailgunID))
	before := automationNames(t, env)

	for name, id := range map[string]int64{"unknown": 999999, "archived": fixtures.IntegrationAcmeMailgunID} {
		_, err := m.Create(ctx, acme, automations.CreateInput{Name: "X", TriggerEvent: "signup", IntegrationID: &id})
		require.ErrorIs(t, err, automations.ErrIntegrationNotFound, name)

		_, err = m.Update(ctx, acme, fixtures.AutomationWelcomeSeriesID, automations.UpdateInput{IntegrationID: &id})
		require.ErrorIs(t, err, automations.ErrIntegrationNotFound, name)
	}
	assert.Equal(t, before, automationNames(t, env), "a refused save stores nothing")
}
