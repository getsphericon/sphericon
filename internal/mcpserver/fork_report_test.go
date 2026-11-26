package mcpserver_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An agent reads each fork path's Enrollments and goals in the report (ADR 0124): Bob has no Tag,
// so he takes the Branch's otherwise.
func TestMCPReportGivesEachForkPath(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	vip, otherwise := automationstep.NewID(), automationstep.NewID()
	branch := automationstep.Step{ID: automationstep.NewID(), Payload: automationstep.Branch{
		Paths: []automationstep.BranchPath{{ID: vip, Condition: conditiontree.Branch{
			Kind: conditiontree.KindGroup, Combinator: conditiontree.And,
			Entries: []json.RawMessage{json.RawMessage(`{"kind":"tag","name":"VIP","operator":"has"}`)},
		}, Steps: []automationstep.Step{}}},
		Otherwise: automationstep.Path{ID: otherwise, Steps: []automationstep.Step{}},
	}}
	root := automationstep.Root(branch)
	a, err := m.Create(ctx, acme, automations.CreateInput{Name: "Forking", Trigger: automations.EventTrigger{Matcher: eventmatcher.Matcher{Action: "fork.entered"}}, Steps: &root})
	require.NoError(t, err)
	_, err = m.Publish(ctx, acme, a.ID, testhelper.AutomationRevisionIn(ctx, t, acme, a.ID))
	require.NoError(t, err)
	stepper := testhelper.Module[*automations.StepExecution](env)
	err = stepper.OnEvent(ctx, acme, automations.OnEventInput{ContactID: fixtures.ContactBobID, Event: automations.Event{Action: "fork.entered", OccurredAt: time.Now()}})
	require.NoError(t, err)
	env.RunQueuedSteps(t, stepper)
	s := env.MCPClient(t, env.ScopedBearer(t, apitokens.ScopeAutomationsRead))

	res := call(t, s, "automations_report", map[string]any{"id": testhelper.ID(a.ID)})

	require.False(t, res.IsError, text(t, res))
	paths := decode(t, text(t, res))["paths"]
	assert.Equal(t, []any{map[string]any{
		"stepId": string(branch.ID), "pathId": string(otherwise), "enrollments": float64(1), "goalReached": float64(0),
	}}, paths)
}
