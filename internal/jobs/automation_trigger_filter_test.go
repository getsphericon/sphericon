package jobs_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Loaded history and a backfilled Event that pass a Trigger's filter still enroll no one, while a
// live one of the same properties does (ADR 0133): the filter adds to what triggers, it never
// lifts the rule that history starts nothing.
func TestAHistoricEventPassingTheTriggerFilterEnrollsNoOne(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	leaf, err := json.Marshal(map[string]any{
		"kind": "property", "key": "status", "test": map[string]any{"kind": "text", "operator": "eq", "value": "paid"},
	})
	require.NoError(t, err)
	filter := &conditiontree.EventFilter{Kind: conditiontree.KindGroup, Combinator: conditiontree.And, Entries: []json.RawMessage{leaf}}
	mod := testhelper.Module[*automations.Module](env)
	a, err := mod.Create(ctx, acme, automations.CreateInput{Name: "Paid orders", Trigger: automations.Trigger{Event: eventmatcher.Matcher{Action: "order.completed", Filter: filter}}})
	require.NoError(t, err)
	_, err = mod.Publish(ctx, acme, a.ID, testhelper.AutomationRevisionIn(ctx, t, acme, a.ID))
	require.NoError(t, err)
	paidOrder := func(contactID int64, backfill bool) *events.CollectedEvent {
		return &events.CollectedEvent{WorkspaceID: fixtures.AcmeID, ContactID: contactID, Action: "order.completed",
			Properties: map[string]any{"status": "paid"}, OccurredAt: time.Now(), Backfill: backfill}
	}

	publishEvent(t, env, paidOrder(fixtures.ContactBobID, true))
	historyTriggers := triggersOn(t, env, "order.completed")
	publishEvent(t, env, paidOrder(fixtures.ContactAliceID, false))
	live := triggersOn(t, env, "order.completed")
	require.Len(t, live, 1)
	require.NoError(t, jobs.NewEvaluateTriggerWorker(env.DB, testhelper.Module[*automations.StepExecution](env)).Work(ctx, job(live[0])))

	assert.Empty(t, historyTriggers, "a backfilled Event queues no evaluation")
	runs, err := env.DB.AutomationRun.Query().Where(automationrun.AutomationID(a.ID)).All(ctx)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	assert.EqualValues(t, fixtures.ContactAliceID, runs[0].ContactID)
}
