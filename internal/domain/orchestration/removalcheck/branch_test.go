package removalcheck_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/liveuse"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/removalcheck"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// branchOn is a root path whose Branch has one path, under the Condition holding the leaf given,
// and an empty otherwise.
func branchOn(t *testing.T, leaf map[string]any) automationstep.Path {
	t.Helper()
	raw, err := json.Marshal(leaf)
	require.NoError(t, err)
	cond := conditiontree.Branch{Kind: conditiontree.KindGroup, Combinator: conditiontree.And, Entries: []json.RawMessage{raw}}
	return automationstep.Root(automationstep.New(automationstep.Branch{
		Paths:     []automationstep.BranchPath{{Condition: cond, Steps: []automationstep.Step{}}},
		Otherwise: automationstep.Path{Steps: []automationstep.Step{}},
	}))
}

// branchingOn makes an Automation of the Workspace whose steps are steps, live when published and
// only in its draft otherwise, and returns it.
func branchingOn(t *testing.T, env *testhelper.TestEnv, workspaceID int64, name string, steps automationstep.Path, published bool) *ent.Automation {
	t.Helper()
	ctx := t.Context()
	s := env.DB.Scoped(workspaceID)
	m := testhelper.Module[*automations.Module](env)
	a, err := m.Create(ctx, s, automations.CreateInput{Name: name, Trigger: automations.EventTrigger{Matcher: eventmatcher.Matcher{Action: "signup"}}, Steps: &steps})
	require.NoError(t, err)
	if published {
		a, err = m.Publish(ctx, s, a.ID, testhelper.AutomationRevisionIn(ctx, t, s, a.ID))
		require.NoError(t, err)
	}
	return a
}

// A Tag a Branch's Condition tests, in a live version or a draft, is in Definition use: its
// removal is refused, naming the Automations.
func TestATagABranchConditionTestsIsRefusedNamingTheAutomation(t *testing.T) {
	env := testhelper.Setup(t)
	tests := map[string]any{"kind": "tag", "name": fixtures.TagUnusedName, "operator": "has"}
	live := branchingOn(t, env, fixtures.AcmeID, "Branch on unused", branchOn(t, tests), true)
	draft := branchingOn(t, env, fixtures.AcmeID, "Draft branch on unused", branchOn(t, tests), false)

	refused := underLock(t, env, fixtures.AcmeID, func(s *ent.WorkspaceLocked) error {
		return testhelper.Module[*removalcheck.Module](env).Tag(t.Context(), s, fixtures.TagUnusedName)
	})

	assert.Equal(t, &liveuse.Error{What: liveuse.SubjectTag, Users: []liveuse.User{
		{Kind: liveuse.Automation, ID: live.ID, Name: live.Name},
		{Kind: liveuse.Automation, ID: draft.ID, Name: draft.Name},
	}}, refused)
}

// A Custom field a Branch's Condition tests is held against a retype as a Rule's Condition holds
// it: a retype to a type that does not take the test is refused, naming the Automation, and one
// that takes it is not.
func TestACustomFieldABranchConditionTestsHoldsItsTypeAsARulesDoes(t *testing.T) {
	env := testhelper.Setup(t)
	eq := map[string]any{"kind": "customField", "key": fixtures.CustomFieldAcmePlanKey, "test": map[string]any{"kind": "text", "operator": "eq", "value": "gold"}}
	a := branchingOn(t, env, fixtures.AcmeID, "Branch on plan", branchOn(t, eq), true)

	toNumber := underLock(t, env, fixtures.AcmeID, func(s *ent.WorkspaceLocked) error {
		return testhelper.Module[*removalcheck.Module](env).CustomFieldRetype(t.Context(), s, fixtures.CustomFieldAcmePlanKey, customfield.TypeDatetime)
	})
	toText := underLock(t, env, fixtures.AcmeID, func(s *ent.WorkspaceLocked) error {
		return testhelper.Module[*removalcheck.Module](env).CustomFieldRetype(t.Context(), s, fixtures.CustomFieldAcmePlanKey, customfield.TypeString)
	})

	holder := liveuse.User{Kind: liveuse.Automation, ID: a.ID, Name: a.Name}
	assert.Contains(t, usersOf(toNumber), holder)
	assert.NotContains(t, usersOf(toText), holder, "a text test still compiles on a text field")
}

// A Custom field a Branch's event test names in its filter, the `key` of contact.field_changed
// (ADR 0153), is held against any retype, even to a type every test of it takes.
func TestACustomFieldABranchsEventFilterNamesHoldsItAgainstAnyRetype(t *testing.T) {
	env := testhelper.Setup(t)
	changed := map[string]any{"kind": "event", "action": "contact.field_changed", "operator": "atLeast", "times": 1, "filter": map[string]any{
		"kind": "group", "combinator": "and", "entries": []any{map[string]any{
			"kind": "property", "key": "key", "test": map[string]any{"kind": "text", "operator": "eq", "value": fixtures.CustomFieldAcmePlanKey},
		}},
	}}
	a := branchingOn(t, env, fixtures.AcmeID, "Branch on a plan change", branchOn(t, changed), true)

	toText := underLock(t, env, fixtures.AcmeID, func(s *ent.WorkspaceLocked) error {
		return testhelper.Module[*removalcheck.Module](env).CustomFieldRetype(t.Context(), s, fixtures.CustomFieldAcmePlanKey, customfield.TypeString)
	})

	assert.Contains(t, usersOf(toText), liveuse.User{Kind: liveuse.Automation, ID: a.ID, Name: a.Name})
}

// usersOf is who a refusal names; none for no refusal.
func usersOf(err error) []liveuse.User {
	refused, _ := errors.AsType[*liveuse.Error](err)
	if refused == nil {
		return nil
	}
	return refused.Users
}
