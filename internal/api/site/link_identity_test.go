package site_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func setLinkIdentity(t *testing.T, env *testhelper.TestEnv, c *siteapi.Client, enabled bool, hosts ...string) (*siteapi.SiteWorkspaceResource, error) {
	t.Helper()
	return c.SiteWorkspacesSetLinkIdentity(t.Context(),
		&siteapi.SiteSetLinkIdentityInput{Revision: acmeRevision(t, env), LinkIdentityEnabled: enabled, LinkIdentityHosts: toLinkIdentityHosts(hosts)},
		siteapi.SiteWorkspacesSetLinkIdentityParams{WorkspaceId: fixtures.AcmePublicID})
}

func toLinkIdentityHosts(hosts []string) []siteapi.LinkIdentityHost {
	out := make([]siteapi.LinkIdentityHost, 0, len(hosts))
	for _, h := range hosts {
		out = append(out, siteapi.LinkIdentityHost(h))
	}
	return out
}

// Link identity tokens are off, with no site listed, until the Workspace turns them on (ADR 0160).
func TestLinkIdentityTokensAreOffUntilTurnedOn(t *testing.T) {
	env := testhelper.Setup(t)

	got, err := env.SiteActor(t, fixtures.MemberMaryEmail).SiteWorkspacesGet(t.Context(), siteapi.SiteWorkspacesGetParams{WorkspaceId: fixtures.AcmePublicID})

	require.NoError(t, err)
	assert.False(t, got.LinkIdentityEnabled)
	assert.Empty(t, got.LinkIdentityHosts)
}

// An Owner turns them on for the Workspace's sites, each host normalized as a Sending domain is, and
// the Workspace reads them on /site and /api.
func TestAnOwnerTurnsOnLinkIdentityTokensForTheWorkspacesSites(t *testing.T) {
	env := testhelper.Setup(t)

	set, err := setLinkIdentity(t, env, env.SiteActor(t, fixtures.OwnerJohnEmail), true, " Shop.Example.COM. ", "example.com", "shop.example.com")

	require.NoError(t, err)
	assert.True(t, set.LinkIdentityEnabled)
	assert.Equal(t, []string{"shop.example.com", "example.com"}, set.LinkIdentityHosts)
	read, err := env.ExternalScoped(t, apitokens.ScopeContactsRead).WorkspaceGet(t.Context())
	require.NoError(t, err)
	assert.True(t, read.LinkIdentityEnabled)
	assert.Equal(t, []string{"shop.example.com", "example.com"}, read.LinkIdentityHosts)
}

// Turning them off keeps the list.
func TestTurningLinkIdentityTokensOffKeepsTheSites(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	_, err := setLinkIdentity(t, env, owner, true, "example.com")
	require.NoError(t, err)

	set, err := setLinkIdentity(t, env, owner, false, "example.com")

	require.NoError(t, err)
	assert.False(t, set.LinkIdentityEnabled)
	assert.Equal(t, []string{"example.com"}, set.LinkIdentityHosts)
}

// The switch cannot be on with no site listed.
func TestLinkIdentityTokensCannotBeOnWithoutASite(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := setLinkIdentity(t, env, env.SiteActor(t, fixtures.OwnerJohnEmail), true)

	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
}

// A host that is not a domain name is refused.
func TestALinkIdentityHostMustBeADomainName(t *testing.T) {
	env := testhelper.Setup(t)

	for _, host := range []string{"localhost", "203.0.113.7", "https://example.com/"} {
		t.Run(host, func(t *testing.T) {
			_, err := setLinkIdentity(t, env, env.SiteActor(t, fixtures.OwnerJohnEmail), true, host)

			testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
		})
	}
}

// A Member may not change them.
func TestAMemberCannotSetLinkIdentityTokens(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := setLinkIdentity(t, env, env.SiteActor(t, fixtures.MemberMaryEmail), true, "example.com")

	testhelper.SiteRefusal(t, err, http.StatusForbidden)
}
