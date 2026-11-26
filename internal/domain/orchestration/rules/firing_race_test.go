package rules_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/rule"
	"github.com/getsphericon/sphericon/ent/rulefiring"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A Rule deleted after Evaluate read it as active records no Firing: the Firing locks the Rule's
// row and checks it again under the lock, so it waits for the Delete and then finds the Rule gone
// (ADR 0098: a deleted Rule never fires again).
func TestARuleDeletedAfterEvaluateReadItRecordsNoFiring(t *testing.T) {
	ctx := t.Context()
	env := testhelper.Setup(t, testhelper.Committed())
	client := env.DB
	s := client.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*rules.Module](env)

	// The test holds the Rule's row, so the Delete waits for it, and Evaluate, which reads the Rule
	// before it fires, reaches its Firing while the Delete is still waiting.
	tx, err := client.Tx(ctx)
	require.NoError(t, err)
	_, err = tx.Rule.Query().Where(rule.ID(fixtures.RuleTagClickedID)).ForUpdate().Only(ctx)
	require.NoError(t, err)
	remove := testhelper.Writer{Name: "delete", Run: func() error { return m.Delete(ctx, s, fixtures.RuleTagClickedID) }}
	fire := testhelper.Writer{Name: "evaluate", Run: func() error {
		_, err := m.Evaluate(ctx, s, rules.Event{
			ContactID: fixtures.ContactAliceID, Action: "rule_click", SourceID: "evt-deleted", RecordedAt: time.Now(),
		})
		return err
	}}

	errs := env.QueueBehind(t, func() { _ = tx.Rollback() }, remove, fire)

	require.Equal(t, map[string]error{"delete": nil, "evaluate": nil}, errs)
	fired, err := client.RuleFiring.Query().
		Where(rulefiring.RuleID(fixtures.RuleTagClickedID), rulefiring.SourceID("evt-deleted")).
		Exist(ctx)
	require.NoError(t, err)
	assert.False(t, fired, "a deleted Rule fires no more")
}
