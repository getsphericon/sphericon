package automations_test

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestDecodeReadsTheStoredDefinition(t *testing.T) {
	steps, err := automations.Decode(`[{"type":"email","subject":"Hi","body":"<mjml/>"},{"type":"wait","seconds":60},{"type":"apply_tag","tag":"vip"}]`)
	require.NoError(t, err)
	assert.Equal(t, []automations.Step{
		{Type: automations.StepEmail, Subject: "Hi", Body: "<mjml/>"},
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

func TestCreateStoresADraftWithEncodedSteps(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher)
	ctx := context.Background()

	a, err := m.Create(ctx, env.DB.Scoped(fixtures.AcmeID), automations.CreateInput{
		Name: "Onboarding", TriggerEvent: "signup",
		Steps: []automations.Step{{Type: automations.StepApplyTag, Tag: " new "}, {Type: automations.StepWait, Seconds: 5}},
	})
	require.NoError(t, err)
	assert.Equal(t, automation.StatusDraft, a.Status)
	assert.EqualValues(t, fixtures.AcmeID, a.WorkspaceID)
	steps, err := automations.Decode(a.Definition)
	require.NoError(t, err)
	assert.Equal(t, []automations.Step{{Type: automations.StepApplyTag, Tag: "new"}, {Type: automations.StepWait, Seconds: 5}}, steps)

	bare, err := m.Create(ctx, env.DB.Scoped(fixtures.AcmeID), automations.CreateInput{Name: "No steps", TriggerEvent: "x"})
	require.NoError(t, err)
	none, err := automations.Decode(bare.Definition)
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestInvalidStepsAreRefusedAndNothingIsStored(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher)
	ctx := context.Background()
	before, err := env.DB.Automation.Query().Count(ctx)
	require.NoError(t, err)

	for name, steps := range map[string][]automations.Step{
		"unknown type":        {{Type: "sms"}},
		"negative wait":       {{Type: automations.StepWait, Seconds: -1}},
		"apply_tag blank tag": {{Type: automations.StepApplyTag, Tag: "  "}},
		"remove_tag no tag":   {{Type: automations.StepRemoveTag}},
	} {
		_, err := m.Create(ctx, env.DB.Scoped(fixtures.AcmeID), automations.CreateInput{Name: "x", TriggerEvent: "y", Steps: steps})
		assert.ErrorIs(t, err, automations.ErrInvalidStep, name)

		_, err = m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationWelcomeSeriesID, automations.UpdateInput{Steps: &steps})
		assert.ErrorIs(t, err, automations.ErrInvalidStep, "update: "+name)
	}

	after, err := env.DB.Automation.Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	welcome, err := m.Get(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationWelcomeSeriesID)
	require.NoError(t, err)
	stored, err := automations.Decode(welcome.Definition)
	require.NoError(t, err)
	assert.Len(t, stored, 3, "a refused update leaves the definition as it was")
}

func TestListIsNewestFirstWithinTheWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	page, err := automations.New(env.Cipher).List(context.Background(), env.DB.Scoped(fixtures.AcmeID), pagination.Params{Page: 1, PageSize: 100})
	require.NoError(t, err)
	require.Greater(t, page.TotalItems, 1)
	for i, a := range page.Items {
		assert.EqualValues(t, fixtures.AcmeID, a.WorkspaceID, "another tenant's automation leaked")
		if i > 0 {
			assert.Greater(t, page.Items[i-1].ID, a.ID, "newest first")
		}
	}
}

func TestUpdateChangesOnlyWhatIsGivenAndKeepsTheStatus(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher)
	ctx := context.Background()
	before, err := m.Get(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationWelcomeSeriesID)
	require.NoError(t, err)

	renamed, err := m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), before.ID, automations.UpdateInput{Name: lo.ToPtr("Renamed"), TriggerEvent: lo.ToPtr("new.trigger")})
	require.NoError(t, err)
	assert.Equal(t, "Renamed", renamed.Name)
	assert.Equal(t, "new.trigger", renamed.TriggerEvent)
	assert.Equal(t, before.Definition, renamed.Definition, "steps untouched when not given")
	assert.Equal(t, before.Status, renamed.Status)

	resteps, err := m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), before.ID, automations.UpdateInput{Steps: &[]automations.Step{{Type: automations.StepWait, Seconds: 1}}})
	require.NoError(t, err)
	steps, err := automations.Decode(resteps.Definition)
	require.NoError(t, err)
	assert.Equal(t, []automations.Step{{Type: automations.StepWait, Seconds: 1}}, steps)
}

func TestActivateAndDeactivateFlipTheStatus(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher)
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
	m := automations.New(env.Cipher)
	ctx := context.Background()
	a, err := m.Create(ctx, env.DB.Scoped(fixtures.AcmeID), automations.CreateInput{Name: "Temp", TriggerEvent: "x"})
	require.NoError(t, err)

	require.NoError(t, m.Delete(ctx, env.DB.Scoped(fixtures.AcmeID), a.ID))
	_, err = m.Get(ctx, env.DB.Scoped(fixtures.AcmeID), a.ID)
	assert.ErrorIs(t, err, automations.ErrNotFound)
	assert.ErrorIs(t, m.Delete(ctx, env.DB.Scoped(fixtures.AcmeID), a.ID), automations.ErrNotFound, "deleting twice")
}

// Every operation is workspace-scoped: another tenant's id is "not found" and the
// row is untouched.
func TestOperationsNeverReachAnotherWorkspacesAutomation(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher)
	ctx := context.Background()
	foreign, err := env.DB.Automation.Create().SetWorkspaceID(fixtures.GlobexID).SetName("Globex flow").SetTriggerEvent("x").Save(ctx)
	require.NoError(t, err)

	_, err = m.Get(ctx, env.DB.Scoped(fixtures.AcmeID), foreign.ID)
	assert.ErrorIs(t, err, automations.ErrNotFound)
	_, err = m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), foreign.ID, automations.UpdateInput{Name: lo.ToPtr("hijacked")})
	assert.ErrorIs(t, err, automations.ErrNotFound)
	_, err = m.Activate(ctx, env.DB.Scoped(fixtures.AcmeID), foreign.ID)
	assert.ErrorIs(t, err, automations.ErrNotFound)
	_, err = m.Deactivate(ctx, env.DB.Scoped(fixtures.AcmeID), foreign.ID)
	assert.ErrorIs(t, err, automations.ErrNotFound)
	assert.ErrorIs(t, m.Delete(ctx, env.DB.Scoped(fixtures.AcmeID), foreign.ID), automations.ErrNotFound)

	_, err = m.Get(ctx, env.DB.Scoped(fixtures.AcmeID), 999999)
	assert.ErrorIs(t, err, automations.ErrNotFound, "an unknown id")

	stored, err := env.DB.Automation.Get(ctx, foreign.ID)
	require.NoError(t, err)
	assert.Equal(t, "Globex flow", stored.Name)
	assert.Equal(t, automation.StatusDraft, stored.Status)
}
