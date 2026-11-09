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
	"github.com/getsphericon/sphericon/internal/domain/identity/admission"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/domain/identity/oauthclients"
	"github.com/getsphericon/sphericon/internal/domain/identity/secondfactors"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
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
	s := newModule(env, env.DB)

	_, err := s.Describe(t.Context(), "no-such-client", fixtureRedirectURI, "")
	require.ErrorIs(t, err, oauthclients.ErrUnknownClient)
	_, err = s.Describe(t.Context(), fixtureClientID, fixtureRedirectURI+"/", "")
	require.ErrorIs(t, err, oauthclients.ErrInvalidRequest, "the redirect_uri must match exactly, not by prefix")
	_, err = s.Describe(t.Context(), fixtureClientID, fixtureRedirectURI, "bogus nonsense")
	require.ErrorIs(t, err, oauthclients.ErrInvalidRequest, "a request for only unsupported scopes grants nothing")

	req, err := s.Describe(t.Context(), fixtureClientID, fixtureRedirectURI, "")
	require.NoError(t, err)
	assert.NotEmpty(t, req.Scopes, "no scope asked means the default grant")

	_, err = newModule(env, closedClient(t)).Describe(t.Context(), fixtureClientID, fixtureRedirectURI, "")
	require.Error(t, err)
	assert.NotErrorIs(t, err, oauthclients.ErrUnknownClient, "an outage is not an unknown client")
}

func TestDecideRecordsOnlyWellFormedApprovals(t *testing.T) {
	env := testhelper.Setup(t)
	s := newModule(env, env.DB)
	good := oauthclients.Decision{
		ClientID: fixtureClientID, RedirectURI: fixtureRedirectURI, State: "st",
		CodeChallenge: oauth2.S256ChallengeFromVerifier(verifier), Scope: "contacts:read", Admission: johnInAcme, Approve: true,
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
	_, err := s.Decide(t.Context(), d)
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

	_, err = newModule(env, closedClient(t)).Decide(t.Context(), good)
	require.Error(t, err)
	assert.NotErrorIs(t, err, oauthclients.ErrUnknownClient)
}

// The User is admitted to the Workspace the decision names before anything else, as every
// operation of a Workspace is (ADR 0061): a Workspace they are no member of, or none, refuses even
// a denial, and a required acceptance pending refuses an approval; no code is left behind.
func TestDecideAdmitsTheUserToTheWorkspaceFirst(t *testing.T) {
	env := testhelper.Setup(t)
	s := newModule(env, env.DB)
	approval := oauthclients.Decision{
		ClientID: fixtureClientID, RedirectURI: fixtureRedirectURI,
		CodeChallenge: oauth2.S256ChallengeFromVerifier(verifier), Scope: "contacts:read", Admission: johnInAcme, Approve: true,
	}
	denial := approval
	denial.Approve = false
	denial.Admission.Workspace = fixtures.UmbrellaPublicID
	unnamed := approval
	unnamed.Admission.Workspace = ""

	_, notMember := s.Decide(t.Context(), denial)
	_, none := s.Decide(t.Context(), unnamed)
	env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)
	_, pending := s.Decide(t.Context(), approval)

	require.ErrorIs(t, notMember, workspaces.ErrNotFound)
	require.ErrorIs(t, none, workspaces.ErrNotFound)
	require.ErrorIs(t, pending, users.ErrAcceptancePending)
	assert.Zero(t, env.DB.OAuthCode.Query().CountX(t.Context()))
}

// A code is spent only by the request it was issued for: a refused claim (another client, another
// redirect URI, a verifier that does not match the challenge) leaves it to the right one, which
// gets the grant once.
func TestClaimCodeChecksTheRequestBeforeItSpendsTheCode(t *testing.T) {
	env := testhelper.Setup(t)
	target, err := newModule(env, env.DB).Decide(t.Context(), oauthclients.Decision{
		ClientID: fixtureClientID, RedirectURI: fixtureRedirectURI,
		CodeChallenge: oauth2.S256ChallengeFromVerifier(verifier), Scope: "contacts:read", Admission: johnInAcme, Approve: true,
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

// johnInAcme is a request of John, the Owner of Acme, on his session, deciding for Acme.
var johnInAcme = admission.Request{UserID: fixtures.OwnerJohnID, Workspace: fixtures.AcmePublicID}

// newModule is the OAuth clients module over client with the production metadata fetcher, which
// admits the User over the test's database.
func newModule(env *testhelper.TestEnv, client *ent.Client) *oauthclients.Module {
	acceptance := users.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords, time.Now)
	admit := admission.New(memberships.New(env.DB, env.Bus), acceptance, secondfactors.New(env.DB, env.Bus, env.Cipher, acceptance, time.Now))
	return oauthclients.New(client, oauthclients.NewFetcher(webhook.Allowlist{}), admit, time.Now)
}
