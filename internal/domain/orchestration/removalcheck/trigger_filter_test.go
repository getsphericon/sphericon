package removalcheck_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/liveuse"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/removalcheck"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// namingTrigger is the Trigger on action whose filter tests that the property key is value.
func namingTrigger(t *testing.T, action, key, value string) eventmatcher.Matcher {
	t.Helper()
	leaf, err := json.Marshal(map[string]any{
		"kind": "property", "key": key, "test": map[string]any{"kind": "text", "operator": "eq", "value": value},
	})
	require.NoError(t, err)
	return eventmatcher.Matcher{Action: action, Filter: &conditiontree.EventFilter{Group: conditiontree.Group{
		Kind: conditiontree.KindGroup, Combinator: conditiontree.And, Entries: []json.RawMessage{leaf},
	}}}
}

// filteredOn makes an Automation of the Workspace whose Trigger is trigger, live when published and
// only in its draft otherwise, and returns it.
func filteredOn(t *testing.T, env *testhelper.TestEnv, workspaceID int64, name string, trigger eventmatcher.Matcher, published bool) *ent.Automation {
	t.Helper()
	ctx := context.Background()
	s := env.DB.Scoped(workspaceID)
	m := testhelper.Module[*automations.Module](env)
	a, err := m.Create(ctx, s, automations.CreateInput{Name: name, Trigger: trigger})
	require.NoError(t, err)
	if published {
		a, err = m.Publish(ctx, s, a.ID, testhelper.AutomationRevisionIn(ctx, t, s, a.ID))
		require.NoError(t, err)
	}
	return a
}

// A Tag a Trigger's filter names, in a live version or a draft, is in Definition use (ADR 0133):
// its removal is refused, naming the Automations.
func TestATagATriggerFilterNamesIsRefusedNamingTheAutomation(t *testing.T) {
	env := testhelper.Setup(t)
	live := filteredOn(t, env, fixtures.AcmeID, "On unused", namingTrigger(t, systemevent.ContactTagged, "tag", fixtures.TagUnusedName), true)
	draft := filteredOn(t, env, fixtures.AcmeID, "Off unused", namingTrigger(t, systemevent.ContactUntagged, "tag", fixtures.TagUnusedName), false)

	refused := underLock(t, env, fixtures.AcmeID, func(s *ent.WorkspaceLocked) error {
		return testhelper.Module[*removalcheck.Module](env).Tag(t.Context(), s, fixtures.TagUnusedName)
	})

	assert.Equal(t, &liveuse.Error{What: liveuse.SubjectTag, Users: []liveuse.User{
		{Kind: liveuse.Automation, ID: live.ID, Name: live.Name},
		{Kind: liveuse.Automation, ID: draft.ID, Name: draft.Name},
	}}, refused)
}

// A Custom field a Trigger's filter names, in a live version or a draft, is in Definition use: a
// Custom field has no delete, so its retype is what is refused, naming the Automations.
func TestACustomFieldATriggerFilterNamesIsRefusedItsRetypeNamingTheAutomation(t *testing.T) {
	env := testhelper.Setup(t)
	live := filteredOn(t, env, fixtures.DuffID, "On has_it", namingTrigger(t, systemevent.ContactFieldChanged, "key", fixtures.CustomFieldDuffHasItKey), true)
	draft := filteredOn(t, env, fixtures.DuffID, "Draft on has_it", namingTrigger(t, systemevent.ContactFieldChanged, "key", fixtures.CustomFieldDuffHasItKey), false)

	refused := underLock(t, env, fixtures.DuffID, func(s *ent.WorkspaceLocked) error {
		return testhelper.Module[*removalcheck.Module](env).CustomFieldRetype(t.Context(), s, fixtures.CustomFieldDuffHasItKey, customfield.TypeString)
	})

	assert.Equal(t, &liveuse.Error{What: liveuse.SubjectCustomField, Users: []liveuse.User{
		{Kind: liveuse.Automation, ID: live.ID, Name: live.Name},
		{Kind: liveuse.Automation, ID: draft.ID, Name: draft.Name},
	}}, refused)
}
