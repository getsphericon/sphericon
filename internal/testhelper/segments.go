package testhelper

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
)

// ConditionInput builds a Condition of a surface's request type C (siteapi.SegmentConditionInput,
// externalapi.RuleConditionInput, …) from its JSON, so a test reads as the tree it sends instead of
// the nested generated union. A kind the place does not admit, or a key a leaf does not declare,
// fails the test here; a value outside an enum decodes, and is the server's to refuse.
func ConditionInput[C any, PC interface {
	*C
	json.Unmarshaler
}](tb testing.TB, def string) C {
	tb.Helper()
	var tree C
	require.NoError(tb, PC(&tree).UnmarshalJSON([]byte(def)), "the Condition is a tree of its place")
	return tree
}

// SegmentCondition is a Segment's Condition a test writes as JSON, the value the modules take and
// the columns hold (ADR 0169). It is read through the contract's closed type first, so a kind a
// Segment does not admit, or a key a leaf does not declare, fails the test.
func SegmentCondition(tb testing.TB, def string) conditiontree.Segment {
	tb.Helper()
	ConditionInput[siteapi.SegmentConditionInput](tb, def)
	var g conditiontree.Segment
	require.NoError(tb, json.Unmarshal([]byte(def), &g))
	return g
}

// RuleCondition is SegmentCondition for a Rule's Condition.
func RuleCondition(tb testing.TB, def string) conditiontree.Rule {
	tb.Helper()
	ConditionInput[siteapi.RuleConditionInput](tb, def)
	var g conditiontree.Rule
	require.NoError(tb, json.Unmarshal([]byte(def), &g))
	return g
}

// Document is v as JSON, so a test compares a stored document with assert.JSONEq: jsonb keeps
// neither key order nor spacing (ADR 0169).
func Document(tb testing.TB, v any) string {
	tb.Helper()
	b, err := json.Marshal(v)
	require.NoError(tb, err)
	return string(b)
}
