package testhelper

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
)

// SiteSegmentCondition builds a site API Segment's Condition from its JSON, so a test reads as the
// tree it sends instead of the nested generated union. A kind the place does not admit, or a key
// a leaf does not declare, fails the test here; a value outside an enum decodes, and is the
// server's to refuse.
func SiteSegmentCondition(tb testing.TB, def string) siteapi.SegmentConditionInput {
	tb.Helper()
	var tree siteapi.SegmentConditionInput
	require.NoError(tb, tree.UnmarshalJSON([]byte(def)))
	return tree
}

// SiteRuleCondition is SiteSegmentCondition for a Rule's Condition.
func SiteRuleCondition(tb testing.TB, def string) siteapi.RuleConditionInput {
	tb.Helper()
	var tree siteapi.RuleConditionInput
	require.NoError(tb, tree.UnmarshalJSON([]byte(def)))
	return tree
}

// ExternalSegmentCondition is SiteSegmentCondition for the external API.
func ExternalSegmentCondition(tb testing.TB, def string) externalapi.SegmentConditionInput {
	tb.Helper()
	var tree externalapi.SegmentConditionInput
	require.NoError(tb, tree.UnmarshalJSON([]byte(def)))
	return tree
}

// ExternalRuleCondition is SiteRuleCondition for the external API.
func ExternalRuleCondition(tb testing.TB, def string) externalapi.RuleConditionInput {
	tb.Helper()
	var tree externalapi.RuleConditionInput
	require.NoError(tb, tree.UnmarshalJSON([]byte(def)))
	return tree
}

// SegmentCondition is a Segment's Condition a test writes as JSON, the value the modules take and
// the columns hold (ADR 0169). It is read through the contract's closed type first, so a kind a
// Segment does not admit, or a key a leaf does not declare, fails the test.
func SegmentCondition(tb testing.TB, def string) conditiontree.Segment {
	tb.Helper()
	SiteSegmentCondition(tb, def)
	var g conditiontree.Segment
	require.NoError(tb, json.Unmarshal([]byte(def), &g))
	return g
}

// RuleCondition is SegmentCondition for a Rule's Condition.
func RuleCondition(tb testing.TB, def string) conditiontree.Rule {
	tb.Helper()
	SiteRuleCondition(tb, def)
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
