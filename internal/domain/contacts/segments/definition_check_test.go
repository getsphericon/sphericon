package segments_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The database refuses a Segment Condition that is no JSON group, whatever code writes it
// (segments_condition_check); an empty Condition, matching every Contact, is kept.
func TestTheDatabaseRefusesAConditionThatIsNoJSON(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)

	// test writes: the refused update is the operation under test.
	err := s.Segment().UpdateOneID(fixtures.SegmentProPlanID).SetCondition("status == 'active'").Exec(t.Context())
	require.Error(t, err)
	assert.True(t, ent.IsConstraintError(err), "a constraint refuses it: %v", err)
	require.NoError(t, s.Segment().UpdateOneID(fixtures.SegmentProPlanID).SetCondition("").Exec(t.Context()))
}
