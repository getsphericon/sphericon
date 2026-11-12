package testhelper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/ent/rule"
	"github.com/getsphericon/sphericon/ent/segment"
	"github.com/getsphericon/sphericon/ent/user"
	collectapi "github.com/getsphericon/sphericon/gen/collect"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	apiauth "github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
	gptoken "github.com/go-pkgz/auth/v2/token"
	"github.com/golang-jwt/jwt/v5"
	ht "github.com/ogen-go/ogen/http"
	"github.com/stretchr/testify/require"
)

// An actor is a ready typed client for one API surface acting as one identity.
// JWT signing, token generation and hashing, security sources and the in-memory
// transport are internal to this file; tests only pick the identity.

const (
	siteBase     = "http://local/site"
	externalBase = "http://local/api"
	collectBase  = "http://local/collect"
)

// SiteActor returns a /site client authenticated as the fixture user with the
// given login email (use the fixtures.*Email constants). Each request carries a
// session token minted for it (see SiteToken); hold a SiteToken for a session
// that must outlive an epoch bump.
func (env *TestEnv) SiteActor(t *testing.T, email string) *siteapi.Client {
	t.Helper()
	c, err := siteapi.NewClient(siteBase, sessionSource{env, email}, siteapi.WithClient(env.Transport(nil)))
	require.NoError(t, err)
	return c
}

// SiteWithToken returns a /site client carrying the given raw session token (for
// tests of tokens in states a login never issues, minted with SiteToken).
func (env *TestEnv) SiteWithToken(t *testing.T, token string) *siteapi.Client {
	t.Helper()
	return env.siteClient(t, token)
}

// SiteActorVia is SiteActor with the in-memory transport wrapped by wrap, for
// tests that observe the wire (e.g. raw response bodies).
func (env *TestEnv) SiteActorVia(t *testing.T, email string, wrap func(inner ht.Client) ht.Client) *siteapi.Client {
	t.Helper()
	c, err := siteapi.NewClient(siteBase, sessionSource{env, email}, siteapi.WithClient(wrap(env.Transport(nil))))
	require.NoError(t, err)
	return c
}

// SiteAnonymous returns a /site client carrying no credential.
func (env *TestEnv) SiteAnonymous(t *testing.T) *siteapi.Client {
	t.Helper()
	return env.siteClient(t, "")
}

// ExternalAnchor returns an /api client bearing the fixture anchor token
// (scopes contacts, tokens and events, read and write).
func (env *TestEnv) ExternalAnchor(t *testing.T) *externalapi.Client {
	t.Helper()
	return env.ExternalWithToken(t, credentials.TokenValue(fixtures.AnchorTokenPrefix, fixtures.AnchorTokenSecret))
}

// ExternalScoped returns an /api client bearing a fresh Acme token that has
// exactly the given scopes, created through the production token code.
func (env *TestEnv) ExternalScoped(t *testing.T, scopes ...string) *externalapi.Client {
	t.Helper()
	return env.ExternalWithToken(t, env.ScopedBearer(t, scopes...))
}

// ScopedBearer returns the raw bearer value of a fresh Acme token that has
// exactly the given scopes, for tests that must speak raw HTTP or MCP.
func (env *TestEnv) ScopedBearer(t *testing.T, scopes ...string) string {
	t.Helper()
	return env.ScopedBearerFor(t, fixtures.AcmeID, scopes...)
}

// ScopedBearerFor is ScopedBearer for another fixture Workspace (tenant-isolation
// tests, e.g. fixtures.GlobexID).
func (env *TestEnv) ScopedBearerFor(t *testing.T, workspaceID int64, scopes ...string) string {
	t.Helper()
	prefix := credentials.GenerateTokenPrefix()
	secret := credentials.GenerateTokenSecret()
	// test writes: a token of exactly the scopes the caller names, through the production hash
	// (internal/AGENTS.md, Actors): no fixture can hold every scope set a test asks for.
	_, err := env.DB.ApiToken.Create().
		SetName("actor-token").SetPrefix(prefix).SetSecretHash(credentials.HashSecret(secret)).SetScopes(scopes).
		SetWorkspaceID(workspaceID).
		Save(t.Context())
	require.NoError(t, err)
	return credentials.TokenValue(prefix, secret)
}

// ExternalAnonymous returns an /api client carrying no credential.
func (env *TestEnv) ExternalAnonymous(t *testing.T) *externalapi.Client {
	t.Helper()
	return env.ExternalWithToken(t, "")
}

// ExternalWithToken returns an /api client bearing the given raw token value
// (for wrong-credential tests).
func (env *TestEnv) ExternalWithToken(t *testing.T, bearer string) *externalapi.Client {
	t.Helper()
	c, err := externalapi.NewClient(externalBase, bearerSource{bearer}, externalapi.WithClient(env.Transport(nil)))
	require.NoError(t, err)
	return c
}

// ExternalVia is ExternalWithToken with the in-memory transport wrapped by wrap, for
// tests that change the request on the wire (e.g. RewriteURL).
func (env *TestEnv) ExternalVia(t *testing.T, bearer string, wrap func(inner ht.Client) ht.Client) *externalapi.Client {
	t.Helper()
	c, err := externalapi.NewClient(externalBase, bearerSource{bearer}, externalapi.WithClient(wrap(env.Transport(nil))))
	require.NoError(t, err)
	return c
}

// RewriteURL wraps a transport so every occurrence of from in a request's URL (path and
// query) becomes to. A typed client cannot spell a malformed id, so a test sends a valid
// sentinel id and swaps it on the wire.
func RewriteURL(from, to string) func(inner ht.Client) ht.Client {
	return func(inner ht.Client) ht.Client {
		return rewriteClient{inner: inner, from: from, to: to}
	}
}

type rewriteClient struct {
	inner    ht.Client
	from, to string
}

func (c rewriteClient) Do(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.URL.Path = strings.ReplaceAll(r.URL.Path, c.from, c.to)
	r.URL.RawPath = ""
	r.URL.RawQuery = strings.ReplaceAll(r.URL.RawQuery, c.from, c.to)
	return c.inner.Do(r)
}

// DropField wraps a transport so the JSON object body of every request loses its top-level field
// name. A typed client cannot leave out a required field, so a test sets it and drops it on the
// wire.
func DropField(name string) func(inner ht.Client) ht.Client {
	return func(inner ht.Client) ht.Client {
		return dropClient{inner: inner, name: name}
	}
}

type dropClient struct {
	inner ht.Client
	name  string
}

func (c dropClient) Do(r *http.Request) (*http.Response, error) {
	if r.Body == nil || r.Body == http.NoBody {
		return c.inner.Do(r)
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("drop %q from a body that is no JSON object: %w", c.name, err)
	}
	delete(body, c.name)
	if raw, err = json.Marshal(body); err != nil {
		return nil, err
	}
	r = r.Clone(r.Context())
	r.Body = io.NopCloser(bytes.NewReader(raw))
	r.ContentLength = int64(len(raw))
	return c.inner.Do(r)
}

// CollectAcme returns a /collect client carrying the Acme collect key.
func (env *TestEnv) CollectAcme(t *testing.T) *collectapi.Client {
	t.Helper()
	return env.CollectWithKey(t, fixtures.AcmeCollectKey)
}

// CollectAnonymous returns a /collect client carrying no key.
func (env *TestEnv) CollectAnonymous(t *testing.T) *collectapi.Client {
	t.Helper()
	return env.CollectWithKey(t, "")
}

// CollectWithKey returns a /collect client carrying the given raw key (for
// wrong-credential tests).
func (env *TestEnv) CollectWithKey(t *testing.T, key string) *collectapi.Client {
	t.Helper()
	c, err := collectapi.NewClient(collectBase, collectKeySource{key}, collectapi.WithClient(env.Transport(nil)))
	require.NoError(t, err)
	return c
}

func (env *TestEnv) siteClient(t *testing.T, jwtValue string) *siteapi.Client {
	t.Helper()
	c, err := siteapi.NewClient(siteBase, cookieSource{jwtValue}, siteapi.WithClient(env.Transport(nil)))
	require.NoError(t, err)
	return c
}

// SiteToken mints the session token a login issues for the fixture user with the
// given email: the go-pkgz claims, stamped by the production apiauth.StampUser
// (User id and current session epoch), signed with the test config's secret and
// valid for SPHERICON_SESSION_TTL from the env's clock. edit, when not nil, changes the
// claims before signing, for tests of tokens a login never issues (expired,
// without an epoch). The token is fixed at minting: a later epoch bump ends it.
func (env *TestEnv) SiteToken(t *testing.T, email string, edit func(*gptoken.Claims)) string {
	t.Helper()
	tk, err := env.mintSiteToken(t.Context(), email, edit)
	require.NoError(t, err)
	return tk
}

func (env *TestEnv) mintSiteToken(ctx context.Context, email string, edit func(*gptoken.Claims)) (string, error) {
	svc := gptoken.NewService(gptoken.Opts{
		SecretReader: gptoken.SecretFunc(func(string) (string, error) { return env.jwtSecret, nil }),
		Issuer:       "sphericon",
		DisableXSRF:  true,
	})
	claims := gptoken.Claims{
		Issuer:    "sphericon",
		Audience:  jwt.ClaimStrings{"sphericon"},
		ExpiresAt: jwt.NewNumericDate(env.now().Add(env.sessionTTL)),
		User:      &gptoken.User{Name: email, ID: "test"},
	}
	// An unknown email leaves the claims unstamped: the site rejects the token.
	if u, err := env.DB.User.Query().Where(user.Email(email)).Only(ctx); err == nil {
		apiauth.StampUser(&claims, u)
		claims.User.SetStrAttr(apiauth.ClaimMethod, string(apiauth.MethodPassword))
	} else if !ent.IsNotFound(err) {
		return "", err
	}
	if edit != nil {
		edit(&claims)
	}
	return svc.Token(claims)
}

// sessionSource signs an actor in afresh on every request, under the request's
// context, so the actor always holds a current session.
type sessionSource struct {
	env   *TestEnv
	email string
}

func (s sessionSource) ApiKeyAuth(ctx context.Context, _ siteapi.OperationName) (siteapi.ApiKeyAuth, error) {
	tk, err := s.env.mintSiteToken(ctx, s.email, nil)
	return siteapi.ApiKeyAuth{APIKey: tk}, err
}

type cookieSource struct{ token string }

func (s cookieSource) ApiKeyAuth(context.Context, siteapi.OperationName) (siteapi.ApiKeyAuth, error) {
	return siteapi.ApiKeyAuth{APIKey: s.token}, nil
}

type bearerSource struct{ token string }

func (s bearerSource) BearerAuth(context.Context, externalapi.OperationName) (externalapi.BearerAuth, error) {
	return externalapi.BearerAuth{Token: s.token}, nil
}

type collectKeySource struct{ key string }

func (s collectKeySource) ApiKeyAuth(context.Context, collectapi.OperationName) (collectapi.ApiKeyAuth, error) {
	return collectapi.ApiKeyAuth{APIKey: s.key}, nil
}

// AutomationRevision is the stored revision of the Automation id (ADR 0119): what an editor that
// read it now names in an edit, Publish or Revert.
func (env *TestEnv) AutomationRevision(ctx context.Context, tb testing.TB, id int64) int64 {
	tb.Helper()
	a, err := env.DB.Automation.Get(ctx, id)
	if ent.IsNotFound(err) {
		// An id the call is refused for (another Workspace's, none at all): any revision does.
		return 1
	}
	require.NoError(tb, err)
	return a.Revision
}

// RuleRevision is the stored revision of the Rule id (ADR 0119): what an editor that read it now
// names in an edit, Publish or Revert.
func (env *TestEnv) RuleRevision(ctx context.Context, tb testing.TB, id int64) int64 {
	tb.Helper()
	r, err := env.DB.Rule.Get(ctx, id)
	if ent.IsNotFound(err) {
		// An id the call is refused for (another Workspace's, none at all): any revision does.
		return 1
	}
	require.NoError(tb, err)
	return r.Revision
}

// SegmentRevision is the stored revision of the Segment id (ADR 0119): what an editor that read it
// now names in an edit.
func (env *TestEnv) SegmentRevision(ctx context.Context, tb testing.TB, id int64) int64 {
	tb.Helper()
	sg, err := env.DB.Segment.Get(ctx, id)
	if ent.IsNotFound(err) {
		// An id the call is refused for (another Workspace's, none at all): any revision does.
		return 1
	}
	require.NoError(tb, err)
	return sg.Revision
}

// BroadcastRevision is the stored revision of the Broadcast id (ADR 0119): what an editor that read it
// now names in an edit, a Send or a Schedule.
func (env *TestEnv) BroadcastRevision(ctx context.Context, tb testing.TB, id int64) int64 {
	tb.Helper()
	v, err := env.DB.Broadcast.Get(ctx, id)
	if ent.IsNotFound(err) {
		// An id the call is refused for (another Workspace's, none at all): any revision does.
		return 1
	}
	require.NoError(tb, err)
	return v.Revision
}

// ContactRevision is the stored revision of the Contact id (ADR 0119): what an editor that read it
// now names in an edit.
func (env *TestEnv) ContactRevision(ctx context.Context, tb testing.TB, id int64) int64 {
	tb.Helper()
	v, err := env.DB.Contact.Get(ctx, id)
	if ent.IsNotFound(err) {
		// An id the call is refused for (another Workspace's, none at all): any revision does.
		return 1
	}
	require.NoError(tb, err)
	return v.Revision
}

// CustomFieldRevision is the stored revision of the CustomField id (ADR 0119): what an editor that read it
// now names in an edit.
func (env *TestEnv) CustomFieldRevision(ctx context.Context, tb testing.TB, id int64) int64 {
	tb.Helper()
	v, err := env.DB.CustomField.Get(ctx, id)
	if ent.IsNotFound(err) {
		// An id the call is refused for (another Workspace's, none at all): any revision does.
		return 1
	}
	require.NoError(tb, err)
	return v.Revision
}

// WebhookRevision is the stored revision of the Webhook id (ADR 0119): what an editor that read it
// now names in an edit.
func (env *TestEnv) WebhookRevision(ctx context.Context, tb testing.TB, id int64) int64 {
	tb.Helper()
	v, err := env.DB.WebhookEndpoint.Get(ctx, id)
	if ent.IsNotFound(err) {
		// An id the call is refused for (another Workspace's, none at all): any revision does.
		return 1
	}
	require.NoError(tb, err)
	return v.Revision
}

// IntegrationRevision is the stored revision of the Integration id (ADR 0119): what an editor that
// read it now names in an edit.
func (env *TestEnv) IntegrationRevision(ctx context.Context, tb testing.TB, id int64) int64 {
	tb.Helper()
	v, err := env.DB.Integration.Get(ctx, id)
	if ent.IsNotFound(err) {
		// An id the call is refused for (another Workspace's, none at all): any revision does.
		return 1
	}
	require.NoError(tb, err)
	return v.Revision
}

// AutomationRevisionIn is AutomationRevision read through the scope s, for a test with no TestEnv
// at hand.
func AutomationRevisionIn(ctx context.Context, tb testing.TB, s *ent.Scoped, id int64) int64 {
	tb.Helper()
	a, err := s.Automation().QueryWithArchived().Where(automation.ID(id)).Only(ctx)
	if ent.IsNotFound(err) {
		return 1
	}
	require.NoError(tb, err)
	return a.Revision
}

// RuleRevisionIn is RuleRevision read through the scope s, for a test with no TestEnv at hand.
func RuleRevisionIn(ctx context.Context, tb testing.TB, s *ent.Scoped, id int64) int64 {
	tb.Helper()
	r, err := s.Rule().QueryWithDiscarded().Where(rule.ID(id)).Only(ctx)
	if ent.IsNotFound(err) {
		return 1
	}
	require.NoError(tb, err)
	return r.Revision
}

// SegmentRevisionIn is SegmentRevision read through the scope s, for a test with no TestEnv at hand.
func SegmentRevisionIn(ctx context.Context, tb testing.TB, s *ent.Scoped, id int64) int64 {
	tb.Helper()
	sg, err := s.Segment().QueryWithArchived().Where(segment.ID(id)).Only(ctx)
	if ent.IsNotFound(err) {
		return 1
	}
	require.NoError(tb, err)
	return sg.Revision
}

// BroadcastRevisionIn is BroadcastRevision read through the scope s, for a test with no TestEnv at
// hand.
func BroadcastRevisionIn(ctx context.Context, tb testing.TB, s *ent.Scoped, id int64) int64 {
	tb.Helper()
	b, err := s.Broadcast().QueryWithArchived().Where(broadcast.ID(id)).Only(ctx)
	if ent.IsNotFound(err) {
		return 1
	}
	require.NoError(tb, err)
	return b.Revision
}

// ContactRevisionIn is ContactRevision read through the scope s, for a test with no TestEnv at hand.
func ContactRevisionIn(ctx context.Context, tb testing.TB, s *ent.Scoped, id int64) int64 {
	tb.Helper()
	c, err := s.Contact().Get(ctx, id)
	if ent.IsNotFound(err) {
		return 1
	}
	require.NoError(tb, err)
	return c.Revision
}

// CustomFieldRevisionIn is CustomFieldRevision read through the scope s, for a test with no TestEnv
// at hand.
func CustomFieldRevisionIn(ctx context.Context, tb testing.TB, s *ent.Scoped, id int64) int64 {
	tb.Helper()
	v, err := s.CustomField().Get(ctx, id)
	if ent.IsNotFound(err) {
		return 1
	}
	require.NoError(tb, err)
	return v.Revision
}

// WebhookRevisionIn is WebhookRevision read through the scope s, for a test with no TestEnv at hand.
func WebhookRevisionIn(ctx context.Context, tb testing.TB, s *ent.Scoped, id int64) int64 {
	tb.Helper()
	v, err := s.WebhookEndpoint().Get(ctx, id)
	if ent.IsNotFound(err) {
		return 1
	}
	require.NoError(tb, err)
	return v.Revision
}

// IntegrationRevisionIn is IntegrationRevision read through the scope s, for a test with no TestEnv
// at hand.
func IntegrationRevisionIn(ctx context.Context, tb testing.TB, s *ent.Scoped, id int64) int64 {
	tb.Helper()
	v, err := s.Integration().QueryWithDiscarded().Where(integration.ID(id)).Only(ctx)
	if ent.IsNotFound(err) {
		return 1
	}
	require.NoError(tb, err)
	return v.Revision
}
