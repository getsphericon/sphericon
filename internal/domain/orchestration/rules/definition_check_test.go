package rules_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The database refuses a Rule version whose Actions are no JSON array, as a nil list encodes to
// null, whatever code writes them (rule_versions_actions_check).
func TestTheDatabaseRefusesActionsThatAreNoJSONArray(t *testing.T) {
	env := testhelper.Setup(t)

	// test writes: the refused update is the operation under test.
	err := env.DB.Scoped(fixtures.AcmeID).RuleVersion().Update().SetActions(nil).Exec(t.Context())
	require.Error(t, err)
	assert.True(t, ent.IsConstraintError(err), "a constraint refuses it: %v", err)
}

// The database refuses a Rule version's Condition whose group holds no list of entries, the tree a
// release before ADR 0181 writes (rule_versions_condition_entries_check): it would read as the
// empty group, which matches every Contact.
func TestTheDatabaseRefusesARuleConditionWithoutEntries(t *testing.T) {
	env := testhelper.Setup(t)
	var old conditiontree.Rule
	require.NoError(t, json.Unmarshal([]byte(`{"kind":"group","combinator":"and","rules":[]}`), &old))

	// test writes: the refused update is the operation under test.
	err := env.DB.Scoped(fixtures.AcmeID).RuleVersion().Update().SetCondition(old).Exec(t.Context())
	require.Error(t, err)
	assert.True(t, ent.IsConstraintError(err), "a constraint refuses it: %v", err)
}
