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
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A step is stored with its id, its kind and its kind's fields, a tag name and a Telegram step's
// image trimmed, an SMS step's link tracking only when it is off and an email step's editor
// version only with its document.
func TestStoredStepsKeepTheIDAndTheFieldsOfEachStepKind(t *testing.T) {
	def := automations.StoredSteps(automationstep.Path{ID: "p", Steps: []automationstep.Step{
		{ID: "a", Payload: automationstep.Email{Subject: "Hi", Body: "b", EditorVersion: "stray"}},
		{ID: "b", Payload: automationstep.Wait{Seconds: 30}},
		{ID: "c", Payload: automationstep.ApplyTag{Tag: "  vip "}},
		{ID: "d", Payload: automationstep.RemoveTag{Tag: "old"}},
		{ID: "e", Payload: automationstep.SMS{Text: "Hi", LinkTracking: new(true)}},
		{ID: "f", Payload: automationstep.HTTP{Body: "{}", URL: "sealed", ClearSecret: true}},
	}})
	assert.JSONEq(t, `{"id":"p","steps":[
		{"id":"a","type":"email","subject":"Hi","body":"b"},
		{"id":"b","type":"wait","seconds":30},
		{"id":"c","type":"apply_tag","tag":"vip"},
		{"id":"d","type":"remove_tag","tag":"old"},
		{"id":"e","type":"sms","text":"Hi"},
		{"id":"f","type":"http","url":"sealed","body":"{}"}
	]}`, testhelper.Document(t, def))

	none := automations.StoredSteps(automationstep.Path{ID: "p"})
	assert.Equal(t, []automationstep.Step{}, none.Steps, "no steps are an empty list, never null")
}

// An email step's body is stored as it was written (ADR 0118).
func TestStoredStepsKeepAnEmailStepBodyAsWritten(t *testing.T) {
	def := automations.StoredSteps(automationstep.Path{ID: "p", Steps: []automationstep.Step{
		{ID: "a", Payload: automationstep.Email{Subject: "Hi", Body: `<p onclick="track()">Hi {{ first_name }}</p><form action="https://example.com/s"></form>`}},
	}})
	assert.JSONEq(t, `{"id":"p","steps":[{"id":"a","type":"email","subject":"Hi","body":"<p onclick=\"track()\">Hi {{ first_name }}</p><form action=\"https://example.com/s\"></form>"}]}`, testhelper.Document(t, def))
}

func TestCreateStoresAnInactiveAutomationWithEncodedSteps(t *testing.T) {
	env := testhelper.Setup(t)
	m := testhelper.Module[*automations.Module](env)
	ctx := context.Background()

	a, err := m.Create(ctx, env.DB.Scoped(fixtures.AcmeID), automations.CreateInput{
		Name: "Onboarding", Trigger: eventmatcher.Matcher{Action: "signup"},
		Steps: steps(automationstep.ApplyTag{Tag: " new "}, automationstep.Wait{Seconds: 5}),
	})
	require.NoError(t, err)
	assert.Equal(t, automation.StatusInactive, a.Status)
	assert.EqualValues(t, fixtures.AcmeID, a.WorkspaceID)
	assert.Equal(t, []automationstep.Payload{automationstep.ApplyTag{Tag: "new"}, automationstep.Wait{Seconds: 5}}, draftSteps(t, a))
	live, draft := automations.LiveAndDraft(a)
	assert.Nil(t, live, "nothing is live until it is published")
	assert.Equal(t, "signup", draft.TriggerAction)

	bare, err := m.Create(ctx, env.DB.Scoped(fixtures.AcmeID), automations.CreateInput{Name: "No steps", Trigger: eventmatcher.Matcher{Action: "x"}})
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
	m := testhelper.Module[*automations.Module](env)
	ctx := context.Background()
	before := automationNames(t, env)

	const id = "0f6b8d1e-6c2a-4a55-8f7e-3c1d2b4a5e61"
	one := func(p automationstep.Payload) *automationstep.Path {
		path := automationstep.Root(automationstep.Step{ID: id, Payload: p})
		return &path
	}
	// Each refusal names the Step by its id and kind (ADR 0122).
	for name, c := range map[string]struct {
		refused *automationstep.Path
		names   string
	}{
		"no kind":             {refused: &automationstep.Path{Steps: []automationstep.Step{{}}}},
		"negative wait":       {one(automationstep.Wait{Seconds: -1}), "step " + id + " (wait)"},
		"apply_tag blank tag": {one(automationstep.ApplyTag{Tag: "  "}), "step " + id + " (apply_tag)"},
		"remove_tag no tag":   {one(automationstep.RemoveTag{}), "step " + id + " (remove_tag)"},
		"empty telegram":      {one(automationstep.Telegram{}), "step " + id + " (telegram)"},
		"malformed id": {
			&automationstep.Path{Steps: []automationstep.Step{{ID: "step-1", Payload: automationstep.Wait{}}}},
			`step "step-1" (wait)`,
		},
		"duplicate id": {
			&automationstep.Path{Steps: []automationstep.Step{{ID: id, Payload: automationstep.Wait{}}, {ID: id, Payload: automationstep.ApplyTag{Tag: "vip"}}}},
			"step " + id + " (apply_tag)",
		},
	} {
		_, err := m.Create(ctx, env.DB.Scoped(fixtures.AcmeID), automations.CreateInput{Name: "x", Trigger: eventmatcher.Matcher{Action: "y"}, Steps: c.refused})
		require.ErrorIs(t, err, automations.ErrInvalidStep, name)
		require.ErrorContains(t, err, c.names, name)

		_, err = m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationWelcomeSeriesID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationWelcomeSeriesID), Steps: c.refused})
		require.ErrorIs(t, err, automations.ErrInvalidStep, "update: "+name)
	}

	assert.ElementsMatch(t, before, automationNames(t, env), "nothing was stored")
	welcome, err := m.Get(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationWelcomeSeriesID)
	require.NoError(t, err)
	assert.Len(t, draftSteps(t, welcome), 3, "a refused update leaves the steps as they were")
}

func TestListIsNewestFirstWithinTheWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	page, err := testhelper.Module[*automations.Module](env).List(context.Background(), env.DB.Scoped(fixtures.AcmeID), pagination.Params{Page: 1, PageSize: 100}, automations.ListFilter{})
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
	m := testhelper.Module[*automations.Module](env)
	ctx := context.Background()
	before, err := m.Get(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationWelcomeSeriesID)
	require.NoError(t, err)

	renamed, err := m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), before.ID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, env.DB.Scoped(fixtures.AcmeID), before.ID), Name: new("Renamed"), Trigger: &eventmatcher.Matcher{Action: "new.trigger"}})
	require.NoError(t, err)
	assert.Equal(t, "Renamed", renamed.Name)
	_, draft := automations.LiveAndDraft(renamed)
	assert.Equal(t, "new.trigger", draft.TriggerAction)
	assert.Equal(t, draftSteps(t, before), draftSteps(t, renamed), "steps untouched when not given")
	assert.Equal(t, before.Status, renamed.Status)

	resteps, err := m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), before.ID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, env.DB.Scoped(fixtures.AcmeID), before.ID), Steps: steps(automationstep.Wait{Seconds: 1})})
	require.NoError(t, err)
	assert.Equal(t, []automationstep.Payload{automationstep.Wait{Seconds: 1}}, draftSteps(t, resteps))
}

func TestPublishAndDeactivateFlipTheStatus(t *testing.T) {
	env := testhelper.Setup(t)
	m := testhelper.Module[*automations.Module](env)
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
	m := testhelper.Module[*automations.Module](env)
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
	m := testhelper.Module[*automations.Module](env)
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
	m := testhelper.Module[*automations.Module](env)
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
	m := testhelper.Module[*automations.Module](env)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	using := func() []string {
		t.Helper()
		got, err := testhelper.UnderWorkspaceLock(t, env.Bus, acme, func(l *ent.WorkspaceLocked) ([]*ent.Automation, error) {
			return automationsuse.New().UsingTag(ctx, l, fixtures.TagVipName)
		})
		require.NoError(t, err)
		return lo.Map(got, func(a *ent.Automation, _ int) string { return a.Name })
	}

	live := using()
	assert.Equal(t, []string{fixtures.AutomationTagOnEngagementName}, live, "its live version removes the Tag")

	tagging := steps(automationstep.ApplyTag{Tag: fixtures.TagVipName})
	_, err := m.Update(ctx, acme, fixtures.AutomationWinBackID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, acme, fixtures.AutomationWinBackID), Steps: tagging})
	require.NoError(t, err)
	drafted := using()
	assert.ElementsMatch(t, []string{fixtures.AutomationWinBackName, fixtures.AutomationTagOnEngagementName}, drafted, "a draft names it too")

	require.NoError(t, acme.Automation().Archive(ctx, fixtures.AutomationTagOnEngagementID, time.Now()))
	archived := using()
	assert.Equal(t, []string{fixtures.AutomationWinBackName}, archived, "an archived Automation uses nothing")
}

// emailVia is an email step sending through the Integration id names, nil for the default.
func emailVia(id *int64) automationstep.Email {
	return automationstep.Email{Subject: "Hi", Body: "<p>Hello</p>", IntegrationID: id}
}

func TestEachEmailStepSavesTheIntegrationItSendsThrough(t *testing.T) {
	env := testhelper.Setup(t)
	m := testhelper.Module[*automations.Module](env)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	mailgun, smtp := int64(fixtures.IntegrationAcmeMailgunID), int64(fixtures.IntegrationAcmeDefaultID)

	a, err := m.Create(ctx, acme, automations.CreateInput{
		Name: "Onboarding", Trigger: eventmatcher.Matcher{Action: "signup"},
		Steps: steps(emailVia(&mailgun), automationstep.Wait{Seconds: 60}, emailVia(nil)),
	})
	require.NoError(t, err)
	created := draftIntegrations(t, a)

	renamed, err := m.Update(ctx, acme, a.ID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, acme, a.ID), Name: new("Renamed")})
	require.NoError(t, err)
	kept := draftIntegrations(t, renamed)

	moved, err := m.Update(ctx, acme, a.ID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, acme, a.ID), Steps: steps(emailVia(nil), emailVia(&smtp))})
	require.NoError(t, err)
	rewritten := draftIntegrations(t, moved)

	assert.Equal(t, []*int64{&mailgun, nil, nil}, created, "each email step keeps its own; the wait names none")
	assert.Equal(t, created, kept, "an update that leaves the steps alone keeps them")
	assert.Equal(t, []*int64{nil, &smtp}, rewritten)
}

func TestAnAutomationRefusesAStepIntegrationThatIsMissingOrArchived(t *testing.T) {
	env := testhelper.Setup(t)
	m := testhelper.Module[*automations.Module](env)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	require.NoError(t, acme.Integration().Discard(ctx, fixtures.IntegrationAcmeMailgunID, time.Now()))
	before := automationNames(t, env)

	for name, id := range map[string]int64{"unknown": 999999, "archived": fixtures.IntegrationAcmeMailgunID} {
		sending := steps(emailVia(nil), emailVia(&id))
		_, err := m.Create(ctx, acme, automations.CreateInput{Name: "X", Trigger: eventmatcher.Matcher{Action: "signup"}, Steps: sending})
		require.ErrorIs(t, err, automations.ErrIntegrationNotFound, name)

		_, err = m.Update(ctx, acme, fixtures.AutomationWelcomeSeriesID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, acme, fixtures.AutomationWelcomeSeriesID), Steps: sending})
		require.ErrorIs(t, err, automations.ErrIntegrationNotFound, name)
	}
	assert.Equal(t, before, automationNames(t, env), "a refused save stores nothing")
}

// stepIntegrations is the Integration each step of the version names, nil for none.
func stepIntegrations(v *ent.AutomationVersion) []*int64 {
	out := []*int64{}
	for _, st := range automationstep.All(v.Steps) {
		out = append(out, st.Integration())
	}
	return out
}

// draftIntegrations is the Integration each step of the Automation's draft names, nil for none.
func draftIntegrations(t *testing.T, a *ent.Automation) []*int64 {
	t.Helper()
	_, draft := automations.LiveAndDraft(a)
	require.NotNil(t, draft)
	return stepIntegrations(draft)
}
