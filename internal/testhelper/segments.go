package testhelper

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
)

// SiteRuleGroup builds a site API rule group from its JSON, so a test reads as the rule
// it sends instead of the nested generated union.
func SiteRuleGroup(tb testing.TB, def string) siteapi.ConditionGroupInput {
	tb.Helper()
	var group siteapi.ConditionGroupInput
	require.NoError(tb, group.UnmarshalJSON([]byte(def)))
	return group
}

// ExternalRuleGroup is SiteRuleGroup for the external API.
func ExternalRuleGroup(tb testing.TB, def string) externalapi.ConditionGroupInput {
	tb.Helper()
	var group externalapi.ConditionGroupInput
	require.NoError(tb, group.UnmarshalJSON([]byte(def)))
	return group
}

// Condition is the Condition a test writes as JSON, the value the modules take and the columns
// hold (ADR 0169).
func Condition(tb testing.TB, def string) conditiontree.Group {
	tb.Helper()
	var g conditiontree.Group
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
