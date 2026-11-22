package automations_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The database refuses an Automation version whose steps are a path without a list of Steps, as a
// nil list encodes to null, whatever code writes them (automation_versions_steps_check).
func TestTheDatabaseRefusesStepsThatAreNoPath(t *testing.T) {
	env := testhelper.Setup(t)

	// test writes: the refused update is the operation under test.
	err := env.DB.Scoped(fixtures.AcmeID).AutomationVersion().Update().SetSteps(automationstep.Path{ID: automationstep.NewID()}).Exec(t.Context())
	require.Error(t, err)
	assert.True(t, ent.IsConstraintError(err), "a constraint refuses it: %v", err)
}
