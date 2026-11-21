package fixtures_test

import (
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// demoAPIToken is the line of .mise.toml that gives the demo storefront its API token.
var demoAPIToken = regexp.MustCompile(`(?m)^SPHERICON_DEMO_API_TOKEN = "([^"]*)"$`)

// The demo storefront's server calls the external API with SPHERICON_DEMO_API_TOKEN from
// .mise.toml, and what it writes belongs in Brightpath. The value must be Brightpath's "Demo site"
// fixture token, its prefix and a secret that hashes to it: another token is refused, or writes
// into another workspace.
func TestTheDemoStorefrontTokenIsBrightpathsDemoSiteToken(t *testing.T) {
	env := testhelper.Setup(t)

	raw, err := os.ReadFile("../../.mise.toml")
	require.NoError(t, err)
	m := demoAPIToken.FindSubmatch(raw)
	require.NotNil(t, m, ".mise.toml sets SPHERICON_DEMO_API_TOKEN")
	parsed := credentials.ParseToken(string(m[1]))
	require.NotNil(t, parsed, "SPHERICON_DEMO_API_TOKEN parses as an API token")
	demoSite, err := env.DB.ApiToken.Get(t.Context(), fixtures.BrightpathDemoSiteTokenID)
	require.NoError(t, err)

	assert.Equal(t, int64(fixtures.BrightpathID), demoSite.WorkspaceID, "the Demo site token writes into Brightpath")
	assert.Equal(t, demoSite.Prefix, parsed.Prefix, "SPHERICON_DEMO_API_TOKEN has the Demo site token's prefix")
	assert.True(t, credentials.VerifySecret(parsed.Secret, demoSite.SecretHash),
		"SPHERICON_DEMO_API_TOKEN's secret hashes to the Demo site token's")
}
