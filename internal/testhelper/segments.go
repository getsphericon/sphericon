package testhelper

import (
	"testing"

	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/segments"
)

// SiteRuleGroup builds a site API rule group from its JSON, so a test reads as the rule
// it sends instead of the nested generated union.
func SiteRuleGroup(t testing.TB, def string) siteapi.SiteSegmentRuleGroup {
	t.Helper()
	var group siteapi.SiteSegmentRuleGroup
	require.NoError(t, segments.Decode(def, &group))
	return group
}

// ExternalRuleGroup is SiteRuleGroup for the external API.
func ExternalRuleGroup(t testing.TB, def string) externalapi.SegmentRuleGroup {
	t.Helper()
	var group externalapi.SegmentRuleGroup
	require.NoError(t, segments.Decode(def, &group))
	return group
}
