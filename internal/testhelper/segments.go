package testhelper

import (
	"testing"

	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
)

// SiteRuleGroup builds a site API rule group from its JSON, so a test reads as the rule
// it sends instead of the nested generated union.
func SiteRuleGroup(tb testing.TB, def string) siteapi.ConditionGroupInput {
	tb.Helper()
	var group siteapi.ConditionGroupInput
	require.NoError(tb, condition.Decode(def, &group))
	return group
}

// ExternalRuleGroup is SiteRuleGroup for the external API.
func ExternalRuleGroup(tb testing.TB, def string) externalapi.ConditionGroupInput {
	tb.Helper()
	var group externalapi.ConditionGroupInput
	require.NoError(tb, condition.Decode(def, &group))
	return group
}
