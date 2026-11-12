package rules_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The database refuses a Rule version whose Condition is no JSON group or whose Actions are no JSON
// array, whatever code writes them (rule_versions_condition_check, rule_versions_actions_check).
func TestTheDatabaseRefusesADefinitionThatIsNoJSON(t *testing.T) {
	cases := map[string]func(u *ent.RuleVersionScopedUpdate){
		"condition": func(u *ent.RuleVersionScopedUpdate) { u.SetCondition("tag has vip") },
		"actions":   func(u *ent.RuleVersionScopedUpdate) { u.SetActions(`[{"type":`) },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			// test writes: the refused update is the operation under test.
			update := env.DB.Scoped(fixtures.AcmeID).RuleVersion().Update()
			change(update)
			err := update.Exec(t.Context())
			require.Error(t, err)
			assert.True(t, ent.IsConstraintError(err), "a constraint refuses it: %v", err)
		})
	}
}
