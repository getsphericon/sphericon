package automations_test

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations/automationsuse"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automationstep"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestEncodeKeepsOnlyTheFieldsOfEachStepType(t *testing.T) {
	def, err := automations.Encode([]automationstep.Step{
		{Type: automationstep.Email, Subject: "Hi", Body: "b", Seconds: 9, Tag: "stray"},
		{Type: automationstep.Wait, Seconds: 30, Subject: "stray"},
		{Type: automationstep.ApplyTag, Tag: "  vip ", Body: "stray"},
		{Type: automationstep.RemoveTag, Tag: "old"},
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

// An email step's body is stored as it was written (ADR 0118).
func TestEncodeStoresAnEmailStepBodyAsWritten(t *testing.T) {
	def, err := automations.Encode([]automationstep.Step{
		{Type: automationstep.Email, Subject: "Hi", Body: `<p onclick="track()">Hi {{ first_name }}</p><form action="https://example.com/s"></form>`},
	})
	require.NoError(t, err)
	assert.JSONEq(t, `[{"type":"email","subject":"Hi","body":"<p onclick=\"track()\">Hi {{ first_name }}</p><form action=\"https://example.com/s\"></form>"}]`, def)
}

func TestCreateStoresAnInactiveAutomationWithEncodedSteps(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)
	ctx := context.Background()

	a, err := m.Create(ctx, env.DB.Scoped(fixtures.AcmeID), automations.CreateInput{
		Name: "Onboarding", TriggerEvent: "signup",
		Steps: []automationstep.Step{{Type: automationstep.ApplyTag, Tag: " new "}, {Type: automationstep.Wait, Seconds: 5}},
	})
	require.NoError(t, err)
	assert.Equal(t, automation.StatusInactive, a.Status)
	assert.EqualValues(t, fixtures.AcmeID, a.WorkspaceID)
	assert.Equal(t, []automationstep.Step{{Type: automationstep.ApplyTag, Tag: "new"}, {Type: automationstep.Wait, Seconds: 5}}, draftSteps(t, a))
	live, draft := automations.LiveAndDraft(a)
	assert.Nil(t, live, "nothing is live until it is published")
	assert.Equal(t, "signup", draft.TriggerEvent)

	bare, err := m.Create(ctx, env.DB.Scoped(fixtures.AcmeID), automations.CreateInput{Name: "No steps", TriggerEvent: "x"})
	require.NoError(t, err)
	assert.Empty(t, draftSteps(t, bare))
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
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)
	ctx := context.Background()
	before := automationNames(t, env)

	for name, steps := range map[string][]automationstep.Step{
		"unknown type":        {{Type: "sms"}},
		"negative wait":       {{Type: automationstep.Wait, Seconds: -1}},
		"apply_tag blank tag": {{Type: automationstep.ApplyTag, Tag: "  "}},
		"remove_tag no tag":   {{Type: automationstep.RemoveTag}},
	} {
		_, err := m.Create(ctx, env.DB.Scoped(fixtures.AcmeID), automations.CreateInput{Name: "x", TriggerEvent: "y", Steps: steps})
		require.ErrorIs(t, err, automations.ErrInvalidStep, name)

		_, err = m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationWelcomeSeriesID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationWelcomeSeriesID), Steps: &steps})
		require.ErrorIs(t, err, automations.ErrInvalidStep, "update: "+name)
	}

	assert.ElementsMatch(t, before, automationNames(t, env), "nothing was stored")
	welcome, err := m.Get(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationWelcomeSeriesID)
	require.NoError(t, err)
	assert.Len(t, draftSteps(t, welcome), 3, "a refused update leaves the steps as they were")
}

func TestListIsNewestFirstWithinTheWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	page, err := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now).List(context.Background(), env.DB.Scoped(fixtures.AcmeID), pagination.Params{Page: 1, PageSize: 100}, automations.ListFilter{})
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
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)
	ctx := context.Background()
	before, err := m.Get(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationWelcomeSeriesID)
	require.NoError(t, err)

	renamed, err := m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), before.ID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, env.DB.Scoped(fixtures.AcmeID), before.ID), Name: new("Renamed"), TriggerEvent: new("new.trigger")})
	require.NoError(t, err)
	assert.Equal(t, "Renamed", renamed.Name)
	_, draft := automations.LiveAndDraft(renamed)
	assert.Equal(t, "new.trigger", draft.TriggerEvent)
	assert.Equal(t, draftSteps(t, before), draftSteps(t, renamed), "steps untouched when not given")
	assert.Equal(t, before.Status, renamed.Status)

	resteps, err := m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), before.ID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, env.DB.Scoped(fixtures.AcmeID), before.ID), Steps: &[]automationstep.Step{{Type: automationstep.Wait, Seconds: 1}}})
	require.NoError(t, err)
	assert.Equal(t, []automationstep.Step{{Type: automationstep.Wait, Seconds: 1}}, draftSteps(t, resteps))
}

func TestPublishAndDeactivateFlipTheStatus(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)
	ctx := context.Background()

	off, err := m.Deactivate(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationWelcomeSeriesID)
	require.NoError(t, err)
	assert.Equal(t, automation.StatusInactive, off.Status)
	on, err := m.Publish(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationWelcomeSeriesID, testhelper.AutomationRevisionIn(ctx, t, env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationWelcomeSeriesID))
	require.NoError(t, err)
	assert.Equal(t, automation.StatusActive, on.Status)

	stored, err := env.DB.Automation.Get(ctx, fixtures.AutomationWelcomeSeriesID)
	require.NoError(t, err)
	assert.Equal(t, automation.StatusActive, stored.Status)
}

// An Automation is archived, never deleted (ADR 0100): a never-published one too. It is still read
// by id, and archiving it again changes nothing.
func TestArchiveKeepsANeverPublishedAutomation(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)

	archived, err := m.Archive(ctx, s, fixtures.AutomationWinBackID)
	require.NoError(t, err)
	again, err := m.Archive(ctx, s, fixtures.AutomationWinBackID)
	require.NoError(t, err)
	kept, err := m.Get(ctx, s, fixtures.AutomationWinBackID)
	require.NoError(t, err)

	assert.NotNil(t, archived.ArchivedAt)
	assert.Equal(t, archived.ArchivedAt, again.ArchivedAt, "archiving twice")
	assert.Equal(t, archived.ArchivedAt, kept.ArchivedAt)
}

// Every operation is workspace-scoped: another tenant's id is "not found" and the
// row is untouched.
func TestOperationsNeverReachAnotherWorkspacesAutomation(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)
	ctx := context.Background()
	foreign := env.DB.Automation.GetX(ctx, fixtures.AutomationGlobexID)

	_, err := m.Get(ctx, env.DB.Scoped(fixtures.AcmeID), foreign.ID)
	require.ErrorIs(t, err, automations.ErrNotFound)
	_, err = m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), foreign.ID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, env.DB.Scoped(fixtures.AcmeID), foreign.ID), Name: new("hijacked")})
	require.ErrorIs(t, err, automations.ErrNotFound)
	_, err = m.Publish(ctx, env.DB.Scoped(fixtures.AcmeID), foreign.ID, testhelper.AutomationRevisionIn(ctx, t, env.DB.Scoped(fixtures.AcmeID), foreign.ID))
	require.ErrorIs(t, err, automations.ErrNotFound)
	_, err = m.Deactivate(ctx, env.DB.Scoped(fixtures.AcmeID), foreign.ID)
	require.ErrorIs(t, err, automations.ErrNotFound)
	_, err = m.Revert(ctx, env.DB.Scoped(fixtures.AcmeID), foreign.ID, testhelper.AutomationRevisionIn(ctx, t, env.DB.Scoped(fixtures.AcmeID), foreign.ID))
	require.ErrorIs(t, err, automations.ErrNotFound)
	_, err = m.Archive(ctx, env.DB.Scoped(fixtures.AcmeID), foreign.ID)
	require.ErrorIs(t, err, automations.ErrNotFound)

	_, err = m.Get(ctx, env.DB.Scoped(fixtures.AcmeID), 999999)
	require.ErrorIs(t, err, automations.ErrNotFound, "an unknown id")

	stored, err := env.DB.Automation.Get(ctx, foreign.ID)
	require.NoError(t, err)
	assert.Equal(t, fixtures.AutomationGlobexName, stored.Name)
	assert.Equal(t, automation.StatusInactive, stored.Status)
}

// An archived Automation is read-only until it is unarchived (ADR 0100): Publish and Deactivate
// are refused as archived (409), and change nothing.
func TestAnArchivedAutomationIsNeitherPublishedNorDeactivated(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	_, err := m.Archive(ctx, acme, fixtures.AutomationNotifySlackID)
	require.NoError(t, err)
	before, err := m.Get(ctx, acme, fixtures.AutomationNotifySlackID)
	require.NoError(t, err)

	_, err = m.Publish(ctx, acme, fixtures.AutomationNotifySlackID, testhelper.AutomationRevisionIn(ctx, t, acme, fixtures.AutomationNotifySlackID))
	require.ErrorIs(t, err, automations.ErrArchived)
	_, err = m.Deactivate(ctx, acme, fixtures.AutomationNotifySlackID)
	require.ErrorIs(t, err, automations.ErrArchived)

	after, err := m.Get(ctx, acme, fixtures.AutomationNotifySlackID)
	require.NoError(t, err)
	assert.Equal(t, before.Status, after.Status)
}

// UsingTag names the kept Automations whose live version or draft applies or removes the Tag
// (spec #767, "In use"): a draft that names it keeps it, an archived Automation does not.
func TestUsingTagCountsTheLiveVersionAndTheDraftOfKeptAutomations(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	using := func() []string {
		t.Helper()
		got, err := testhelper.UnderWorkspaceLock(t, env.Bus, acme, func(l *ent.WorkspaceLocked) ([]*ent.Automation, error) {
			return automationsuse.UsingTag(ctx, l, fixtures.TagVipName)
		})
		require.NoError(t, err)
		return lo.Map(got, func(a *ent.Automation, _ int) string { return a.Name })
	}

	live := using()
	assert.Equal(t, []string{fixtures.AutomationTagOnEngagementName}, live, "its live version removes the Tag")

	tagging := []automationstep.Step{{Type: automationstep.ApplyTag, Tag: fixtures.TagVipName}}
	_, err := m.Update(ctx, acme, fixtures.AutomationWinBackID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, acme, fixtures.AutomationWinBackID), Steps: &tagging})
	require.NoError(t, err)
	drafted := using()
	assert.ElementsMatch(t, []string{fixtures.AutomationWinBackName, fixtures.AutomationTagOnEngagementName}, drafted, "a draft names it too")

	require.NoError(t, acme.Automation().Archive(ctx, fixtures.AutomationTagOnEngagementID, time.Now()))
	archived := using()
	assert.Equal(t, []string{fixtures.AutomationWinBackName}, archived, "an archived Automation uses nothing")
}

// emailVia is an email step sending through the Integration id names, nil for the default.
func emailVia(id *int64) automationstep.Step {
	return automationstep.Step{Type: automationstep.Email, Subject: "Hi", Body: "<p>Hello</p>", IntegrationID: id}
}

func TestEachEmailStepSavesTheIntegrationItSendsThrough(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	mailgun, smtp := int64(fixtures.IntegrationAcmeMailgunID), int64(fixtures.IntegrationAcmeDefaultID)

	a, err := m.Create(ctx, acme, automations.CreateInput{
		Name: "Onboarding", TriggerEvent: "signup",
		Steps: []automationstep.Step{emailVia(&mailgun), {Type: automationstep.Wait, Seconds: 60}, emailVia(nil)},
	})
	require.NoError(t, err)
	created := draftIntegrations(t, a)

	renamed, err := m.Update(ctx, acme, a.ID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, acme, a.ID), Name: new("Renamed")})
	require.NoError(t, err)
	kept := draftIntegrations(t, renamed)

	moved, err := m.Update(ctx, acme, a.ID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, acme, a.ID), Steps: &[]automationstep.Step{emailVia(nil), emailVia(&smtp)}})
	require.NoError(t, err)
	rewritten := draftIntegrations(t, moved)

	assert.Equal(t, []*int64{&mailgun, nil, nil}, created, "each email step keeps its own; the wait names none")
	assert.Equal(t, created, kept, "an update that leaves the steps alone keeps them")
	assert.Equal(t, []*int64{nil, &smtp}, rewritten)
}

func TestAnAutomationRefusesAStepIntegrationThatIsMissingOrArchived(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	require.NoError(t, acme.Integration().Discard(ctx, fixtures.IntegrationAcmeMailgunID, time.Now()))
	before := automationNames(t, env)

	for name, id := range map[string]int64{"unknown": 999999, "archived": fixtures.IntegrationAcmeMailgunID} {
		steps := []automationstep.Step{emailVia(nil), emailVia(&id)}
		_, err := m.Create(ctx, acme, automations.CreateInput{Name: "X", TriggerEvent: "signup", Steps: steps})
		require.ErrorIs(t, err, automations.ErrIntegrationNotFound, name)

		_, err = m.Update(ctx, acme, fixtures.AutomationWelcomeSeriesID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, acme, fixtures.AutomationWelcomeSeriesID), Steps: &steps})
		require.ErrorIs(t, err, automations.ErrIntegrationNotFound, name)
	}
	assert.Equal(t, before, automationNames(t, env), "a refused save stores nothing")
}

// stepIntegrations is the Integration each step of the version names, nil for none.
func stepIntegrations(v *ent.AutomationVersion) []*int64 {
	return lo.Map(automations.Views(v.Steps), func(view automations.StepView, _ int) *int64 { return view.IntegrationID })
}

// draftIntegrations is the Integration each step of the Automation's draft names, nil for none.
func draftIntegrations(t *testing.T, a *ent.Automation) []*int64 {
	t.Helper()
	_, draft := automations.LiveAndDraft(a)
	require.NotNil(t, draft)
	return stepIntegrations(draft)
}
