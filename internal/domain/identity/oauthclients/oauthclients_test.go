package oauthclients_test

import (
	"database/sql"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/getsphericon/sphericon/ent"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/domain/identity/oauthclients"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The fixture client (fixtures/oauth_clients.yml) and a fixed PKCE verifier.
const (
	fixtureClientID    = "fixture-client"
	fixtureRedirectURI = "https://connector.example/callback"
	verifier           = "0123456789012345678901234567890123456789012"
)

// What cannot be undone or gives a standing way in is never grantable to a connector: no
// token management, no erasure, no bulk export, no audit log (ADR 0016, ADR 0022).
func TestScopesThatStayTokenOnlyAreNotOAuthScopes(t *testing.T) {
	supported := oauthclients.SupportedScopes()
	for _, scope := range []externalapi.ApiTokenScope{
		externalapi.ApiTokenScopeTokensRead, externalapi.ApiTokenScopeTokensWrite,
		externalapi.ApiTokenScopeContactsErase, externalapi.ApiTokenScopeAuditRead,
		externalapi.ApiTokenScopeExportRead,
		externalapi.ApiTokenScopeCapturedMessagesRead, externalapi.ApiTokenScopeCapturedMessagesWrite,
	} {
		assert.NotContains(t, supported, string(scope))
	}
}

func closedClient(t *testing.T) *ent.Client {
	t.Helper()
	sqlDB, err := sql.Open("pgx", "postgres://closed.invalid/none")
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	return entclient.New(sqlDB)
}

func TestDescribeValidatesClientRedirectAndScope(t *testing.T) {
	env := testhelper.Setup(t)
	s := newModule(env.DB)

	_, err := s.Describe(t.Context(), "no-such-client", fixtureRedirectURI, "")
	require.ErrorIs(t, err, oauthclients.ErrUnknownClient)
	_, err = s.Describe(t.Context(), fixtureClientID, fixtureRedirectURI+"/", "")
	require.ErrorIs(t, err, oauthclients.ErrInvalidRequest, "the redirect_uri must match exactly, not by prefix")
	_, err = s.Describe(t.Context(), fixtureClientID, fixtureRedirectURI, "bogus nonsense")
	require.ErrorIs(t, err, oauthclients.ErrInvalidRequest, "a request for only unsupported scopes grants nothing")

	req, err := s.Describe(t.Context(), fixtureClientID, fixtureRedirectURI, "")
	require.NoError(t, err)
	assert.NotEmpty(t, req.Scopes, "no scope asked means the default grant")

	_, err = newModule(closedClient(t)).Describe(t.Context(), fixtureClientID, fixtureRedirectURI, "")
	require.Error(t, err)
	assert.NotErrorIs(t, err, oauthclients.ErrUnknownClient, "an outage is not an unknown client")
}

func TestDecideRecordsOnlyWellFormedApprovals(t *testing.T) {
	env := testhelper.Setup(t)
	s := newModule(env.DB)
	owner, err := env.DB.Membership.Get(t.Context(), fixtures.AcmeOwnerMembershipID)
	require.NoError(t, err)
	good := oauthclients.Decision{
		ClientID: fixtureClientID, RedirectURI: fixtureRedirectURI, State: "st",
		CodeChallenge: oauth2.S256ChallengeFromVerifier(verifier), Scope: "contacts:read", Membership: owner, Approve: true,
	}
	codes := func() int {
		n, err := env.DB.OAuthCode.Query().Count(t.Context())
		require.NoError(t, err)
		return n
	}

	for name, mutate := range map[string]func(*oauthclients.Decision){
		"short challenge":         func(d *oauthclients.Decision) { d.CodeChallenge = "short" },
		"challenge with a plus":   func(d *oauthclients.Decision) { d.CodeChallenge = strings.Repeat("a", 42) + "+" },
		"overlong challenge":      func(d *oauthclients.Decision) { d.CodeChallenge = strings.Repeat("a", 129) },
		"only unsupported scopes": func(d *oauthclients.Decision) { d.Scope = "bogus" },
		"unregistered redirect":   func(d *oauthclients.Decision) { d.RedirectURI = "https://evil.example/cb" },
	} {
		d := good
		mutate(&d)
		target, err := s.Decide(t.Context(), d)
		require.ErrorIs(t, err, oauthclients.ErrInvalidRequest, name)
		assert.Empty(t, target, name)
	}
	assert.Zero(t, codes(), "no refused approval leaves a code behind")

	d := good
	d.ClientID = "no-such-client"
	_, err = s.Decide(t.Context(), d)
	require.ErrorIs(t, err, oauthclients.ErrUnknownClient)

	// A denial needs no PKCE or scope, only a registered client and redirect.
	denied := good
	denied.Approve, denied.CodeChallenge = false, ""
	target, err := s.Decide(t.Context(), denied)
	require.NoError(t, err)
	u, err := url.Parse(target)
	require.NoError(t, err)
	assert.Equal(t, "access_denied", u.Query().Get("error"))
	assert.Equal(t, "st", u.Query().Get("state"))
	assert.Zero(t, codes())

	target, err = s.Decide(t.Context(), good)
	require.NoError(t, err)
	u, err = url.Parse(target)
	require.NoError(t, err)
	assert.NotEmpty(t, u.Query().Get("code"))
	assert.Equal(t, "st", u.Query().Get("state"))
	stored, err := env.DB.OAuthCode.Query().Only(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"contacts:read"}, stored.Scopes)
	assert.EqualValues(t, fixtures.AcmeID, stored.WorkspaceID)
	assert.EqualValues(t, fixtures.AcmeOwnerMembershipID, stored.MembershipID, "the code names the approving Membership")
	assert.WithinDuration(t, time.Now().Add(5*time.Minute), stored.ExpiresAt, 10*time.Second)
	assert.NotContains(t, target, stored.CodeHash, "only the hash is stored")

	_, err = newModule(closedClient(t)).Decide(t.Context(), good)
	require.Error(t, err)
	assert.NotErrorIs(t, err, oauthclients.ErrUnknownClient)
}

// A code is spent only by the request it was issued for: a refused claim (another client, another
// redirect URI, a verifier that does not match the challenge) leaves it to the right one, which
// gets the grant once.
func TestClaimCodeChecksTheRequestBeforeItSpendsTheCode(t *testing.T) {
	env := testhelper.Setup(t)
	owner, err := env.DB.Membership.Get(t.Context(), fixtures.AcmeOwnerMembershipID)
	require.NoError(t, err)
	target, err := newModule(env.DB).Decide(t.Context(), oauthclients.Decision{
		ClientID: fixtureClientID, RedirectURI: fixtureRedirectURI,
		CodeChallenge: oauth2.S256ChallengeFromVerifier(verifier), Scope: "contacts:read", Membership: owner, Approve: true,
	})
	require.NoError(t, err)
	u, err := url.Parse(target)
	require.NoError(t, err)
	good := oauthclients.CodeClaim{
		Code: u.Query().Get("code"), ClientID: fixtureClientID, RedirectURI: fixtureRedirectURI, Verifier: verifier,
	}
	now := time.Now()

	for name, mutate := range map[string]func(*oauthclients.CodeClaim){
		"unknown code":       func(c *oauthclients.CodeClaim) { c.Code = "nope" },
		"another client":     func(c *oauthclients.CodeClaim) { c.ClientID = "someone-else" },
		"another redirect":   func(c *oauthclients.CodeClaim) { c.RedirectURI = fixtureRedirectURI + "/other" },
		"wrong verifier":     func(c *oauthclients.CodeClaim) { c.Verifier = strings.Repeat("a", 43) },
		"malformed verifier": func(c *oauthclients.CodeClaim) { c.Verifier = "short" },
	} {
		c := good
		mutate(&c)
		_, err := oauthclients.ClaimCodeIn(t.Context(), env.DB, c, now)
		require.ErrorIs(t, err, oauthclients.ErrInvalidGrant, name)
	}

	grant, err := oauthclients.ClaimCodeIn(t.Context(), env.DB, good, now)
	require.NoError(t, err, "no refused claim spent the code")
	assert.EqualValues(t, fixtures.AcmeID, grant.WorkspaceID)
	assert.EqualValues(t, fixtures.AcmeOwnerMembershipID, grant.MembershipID)
	assert.Equal(t, []string{"contacts:read"}, grant.Scopes)
	assert.Equal(t, "Fixture Connector", grant.Edges.Client.Name)

	_, err = oauthclients.ClaimCodeIn(t.Context(), env.DB, good, now)
	require.ErrorIs(t, err, oauthclients.ErrInvalidGrant, "a code is spent once")
}

// newModule is the OAuth clients module with the production metadata fetcher.
func newModule(client *ent.Client) *oauthclients.Module {
	return oauthclients.New(client, oauthclients.NewFetcher(webhook.Allowlist{}), time.Now)
}
