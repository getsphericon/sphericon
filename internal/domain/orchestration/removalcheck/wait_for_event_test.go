package removalcheck_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/liveuse"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/removalcheck"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// waitingFor is a root path whose one Step is a Wait for event that awaits what awaited says.
func waitingFor(awaited automationstep.AwaitedEvent) automationstep.Path {
	return automationstep.Root(automationstep.New(automationstep.WaitForEvent{
		Name: "awaited", TimeoutSeconds: 3600, Awaits: awaited,
		Arrived: automationstep.Path{Steps: []automationstep.Step{}}, TimedOut: automationstep.Path{Steps: []automationstep.Step{}},
	}))
}

// leafGroup is the group of the one leaf given.
func leafGroup(t *testing.T, leaf map[string]any) conditiontree.Group {
	t.Helper()
	raw, err := json.Marshal(leaf)
	require.NoError(t, err)
	return conditiontree.Group{Kind: conditiontree.KindGroup, Combinator: conditiontree.And, Entries: []json.RawMessage{raw}}
}

// A Tag a Wait for event's matcher filter names, or its Condition tests, in a live version or a
// draft, is in Definition use (ADR 0124, ADR 0133): its removal is refused, naming the Automations.
func TestATagAWaitForEventNamesIsRefusedNamingTheAutomation(t *testing.T) {
	env := testhelper.Setup(t)
	filter := leafGroup(t, map[string]any{"kind": "property", "key": "tag", "test": map[string]any{"kind": "text", "operator": "eq", "value": fixtures.TagUnusedName}})
	byFilter := waitingFor(automationstep.AwaitedEvent{Matchers: []eventmatcher.Matcher{
		{Action: systemevent.ContactTagged, Filter: &conditiontree.EventFilter{Group: filter}},
	}})
	cond := leafGroup(t, map[string]any{"kind": "tag", "name": fixtures.TagUnusedName, "operator": "has"})
	byCondition := waitingFor(automationstep.AwaitedEvent{
		Matchers: []eventmatcher.Matcher{{Action: "order.created"}}, Condition: &conditiontree.WaitForEvent{Group: cond},
	})
	filtered := branchingOn(t, env, fixtures.AcmeID, "Waits for the unused tag", byFilter, true)
	conditioned := branchingOn(t, env, fixtures.AcmeID, "Waits on the unused tag", byCondition, false)

	refused := underLock(t, env, fixtures.AcmeID, func(s *ent.WorkspaceLocked) error {
		return testhelper.Module[*removalcheck.Module](env).Tag(t.Context(), s, fixtures.TagUnusedName)
	})

	assert.Equal(t, &liveuse.Error{What: liveuse.SubjectTag, Users: []liveuse.User{
		{Kind: liveuse.Automation, ID: filtered.ID, Name: filtered.Name},
		{Kind: liveuse.Automation, ID: conditioned.ID, Name: conditioned.Name},
	}}, refused)
}

// A Custom field a Wait for event's Condition tests is held against a retype as a Branch's
// Condition holds it, and one its matcher's filter names (the `key` of contact.field_changed) is
// held against any retype.
func TestACustomFieldAWaitForEventNamesHoldsItsType(t *testing.T) {
	env := testhelper.Setup(t)
	eq := leafGroup(t, map[string]any{"kind": "customField", "key": fixtures.CustomFieldAcmePlanKey, "test": map[string]any{"kind": "text", "operator": "eq", "value": "gold"}})
	keyed := leafGroup(t, map[string]any{"kind": "property", "key": "key", "test": map[string]any{"kind": "text", "operator": "eq", "value": fixtures.CustomFieldAcmePlanKey}})
	byCondition := branchingOn(t, env, fixtures.AcmeID, "Waits on the plan", waitingFor(automationstep.AwaitedEvent{
		Matchers: []eventmatcher.Matcher{{Action: "order.created"}}, Condition: &conditiontree.WaitForEvent{Group: eq},
	}), true)
	byFilter := branchingOn(t, env, fixtures.AcmeID, "Waits for the plan to change", waitingFor(automationstep.AwaitedEvent{
		Matchers: []eventmatcher.Matcher{{Action: systemevent.ContactFieldChanged, Filter: &conditiontree.EventFilter{Group: keyed}}},
	}), true)

	toText := underLock(t, env, fixtures.AcmeID, func(s *ent.WorkspaceLocked) error {
		return testhelper.Module[*removalcheck.Module](env).CustomFieldRetype(t.Context(), s, fixtures.CustomFieldAcmePlanKey, customfield.TypeString)
	})
	toNumber := underLock(t, env, fixtures.AcmeID, func(s *ent.WorkspaceLocked) error {
		return testhelper.Module[*removalcheck.Module](env).CustomFieldRetype(t.Context(), s, fixtures.CustomFieldAcmePlanKey, customfield.TypeDatetime)
	})

	conditionHolder := liveuse.User{Kind: liveuse.Automation, ID: byCondition.ID, Name: byCondition.Name}
	filterHolder := liveuse.User{Kind: liveuse.Automation, ID: byFilter.ID, Name: byFilter.Name}
	assert.Contains(t, usersOf(toNumber), conditionHolder)
	assert.NotContains(t, usersOf(toText), conditionHolder, "a text test still compiles on a text field")
	assert.Contains(t, usersOf(toText), filterHolder, "a filter naming the field holds it against any retype")
}
