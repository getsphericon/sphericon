package segments_test

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

// The database refuses a Segment's Condition whose group holds no list of entries, the tree a
// release before ADR 0181 writes, whatever code writes it (segments_condition_entries_check): it
// would read as the empty group, which matches every Contact.
func TestTheDatabaseRefusesASegmentConditionWithoutEntries(t *testing.T) {
	env := testhelper.Setup(t)
	var old conditiontree.Segment
	require.NoError(t, json.Unmarshal([]byte(`{"kind":"group","combinator":"and","rules":[]}`), &old))

	// test writes: the refused update is the operation under test.
	err := env.DB.Scoped(fixtures.AcmeID).Segment().Update().SetCondition(old).Exec(t.Context())
	require.Error(t, err)
	assert.True(t, ent.IsConstraintError(err), "a constraint refuses it: %v", err)
}
