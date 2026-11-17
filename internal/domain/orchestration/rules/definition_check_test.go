package rules_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
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
