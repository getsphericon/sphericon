package removalcheck_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/liveuse"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/removalcheck"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// endingOn makes an Automation of the Workspace on a Trigger that names nothing, ended by the exits
// and goal given, live when published and only in its draft otherwise, and returns it.
func endingOn(t *testing.T, env *testhelper.TestEnv, workspaceID int64, name string, exits []eventmatcher.Matcher, goal *eventmatcher.Matcher, published bool) *ent.Automation {
	t.Helper()
	ctx := context.Background()
	s := env.DB.Scoped(workspaceID)
	m := testhelper.Module[*automations.Module](env)
	a, err := m.Create(ctx, s, automations.CreateInput{Name: name, Trigger: eventmatcher.Matcher{Action: "signed_up"}, ExitEvents: exits, GoalEvent: goal})
	require.NoError(t, err)
	if published {
		a, err = m.Publish(ctx, s, a.ID, testhelper.AutomationRevisionIn(ctx, t, s, a.ID))
		require.NoError(t, err)
	}
	return a
}

// A Tag an exit's or the goal's filter names, in a live version or a draft, is in Definition use
// (ADR 0133): its removal is refused, naming the Automations.
func TestATagAnExitOrGoalFilterNamesIsRefusedNamingTheAutomation(t *testing.T) {
	env := testhelper.Setup(t)
	byExit := endingOn(t, env, fixtures.AcmeID, "Exits on unused",
		[]eventmatcher.Matcher{namingTrigger(t, systemevent.ContactUntagged, "tag", fixtures.TagUnusedName)}, nil, true)
	goal := namingTrigger(t, systemevent.ContactTagged, "tag", fixtures.TagUnusedName)
	byGoal := endingOn(t, env, fixtures.AcmeID, "Reaches unused", nil, &goal, false)

	refused := underLock(t, env, fixtures.AcmeID, func(s *ent.WorkspaceLocked) error {
		return testhelper.Module[*removalcheck.Module](env).Tag(t.Context(), s, fixtures.TagUnusedName)
	})

	assert.Equal(t, &liveuse.Error{What: liveuse.SubjectTag, Users: []liveuse.User{
		{Kind: liveuse.Automation, ID: byExit.ID, Name: byExit.Name},
		{Kind: liveuse.Automation, ID: byGoal.ID, Name: byGoal.Name},
	}}, refused)
}

// A Custom field an exit's or the goal's filter names is in Definition use: its retype is refused,
// naming the Automations.
func TestACustomFieldAnExitOrGoalFilterNamesIsRefusedItsRetypeNamingTheAutomation(t *testing.T) {
	env := testhelper.Setup(t)
	changed := namingTrigger(t, systemevent.ContactFieldChanged, "key", fixtures.CustomFieldDuffHasItKey)
	byExit := endingOn(t, env, fixtures.DuffID, "Exits on has_it", []eventmatcher.Matcher{changed}, nil, true)
	byGoal := endingOn(t, env, fixtures.DuffID, "Reaches has_it", nil, &changed, false)

	refused := underLock(t, env, fixtures.DuffID, func(s *ent.WorkspaceLocked) error {
		return testhelper.Module[*removalcheck.Module](env).CustomFieldRetype(t.Context(), s, fixtures.CustomFieldDuffHasItKey, customfield.TypeString)
	})

	assert.Equal(t, &liveuse.Error{What: liveuse.SubjectCustomField, Users: []liveuse.User{
		{Kind: liveuse.Automation, ID: byExit.ID, Name: byExit.Name},
		{Kind: liveuse.Automation, ID: byGoal.ID, Name: byGoal.Name},
	}}, refused)
}
