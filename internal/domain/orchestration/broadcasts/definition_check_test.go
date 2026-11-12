package broadcasts_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The database refuses an armed Broadcast's Condition that is no JSON group, whatever code writes
// it (broadcasts_condition_check).
func TestTheDatabaseRefusesAConditionThatIsNoJSON(t *testing.T) {
	env := testhelper.Setup(t)

	// test writes: the refused update is the operation under test.
	err := env.DB.Scoped(fixtures.AcmeID).Broadcast().UpdateOneID(fixtures.BroadcastScheduledID).SetCondition("[]").Exec(t.Context())
	require.Error(t, err)
	assert.True(t, ent.IsConstraintError(err), "a constraint refuses it: %v", err)
}
