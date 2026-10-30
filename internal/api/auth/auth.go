// Package auth holds the security handlers of the API surfaces (session cookie, API token, collect
// key), the session issuer and the cookies the sign-in flows set.
package auth

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/apitoken"
	"github.com/getsphericon/sphericon/ent/workspace"
	collectapi "github.com/getsphericon/sphericon/gen/collect"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/apitokens"
	"github.com/getsphericon/sphericon/internal/consentrecords"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/ratelimit"
	"github.com/getsphericon/sphericon/internal/secondfactor"
	gptoken "github.com/go-pkgz/auth/v2/token"
	"github.com/ogen-go/ogen/ogenerrors"
	"github.com/samber/lo"
)

// ErrUnauthorized is returned by security handlers when a request is not authorized.
var ErrUnauthorized = errors.New("unauthorized")

// TokenAuth holds the authenticated api-token context for external API requests.
type TokenAuth struct {
	TokenID     int64
	WorkspaceID int64
	Name        string
	Scopes      []string
	// Membership is, for a Connection (ADR 0061), the approving User's Membership as this request
	// loaded it, with its User and Workspace: its current Role bounds what the scopes allow. It is
	// nil for the Workspace's API token.
	Membership *ent.Membership
	// Scoped is the client confined to WorkspaceID. This file is the external
	// API's construction point of it (ADR 0017); read it with TokenScoped.
	Scoped *ent.Scoped
}

type contextKey struct{}

var tokenAuthKey = contextKey{}

func WithTokenAuth(ctx context.Context, auth *TokenAuth) context.Context {
	return context.WithValue(ctx, tokenAuthKey, auth)
}

func GetTokenAuth(ctx context.Context) *TokenAuth {
	v, _ := ctx.Value(tokenAuthKey).(*TokenAuth)
	return v
}

// HasScope reports whether the authenticated token carries scope; no token carries none.
func HasScope(auth *TokenAuth, scope apitokens.Scope) bool {
	if auth == nil {
		return false
	}
	return lo.Contains(auth.Scopes, string(scope))
}

// TokenScoped returns the Workspace-scoped client of the authenticated api token.
// It is nil on an unauthenticated context; the security handler runs before every
// operation that needs a token, so handlers can rely on it. Handlers pass it to the
// domain modules; they never build a *ent.Scoped from an id themselves.
func TokenScoped(ctx context.Context) *ent.Scoped {
	if a := GetTokenAuth(ctx); a != nil {
		return a.Scoped
	}
	return nil
}

// ExternalSecurityHandler implements externalapi.SecurityHandler (Bearer token auth).
type ExternalSecurityHandler struct {
	ent      *ent.Client
	bus      *events.Bus
	consents *consentrecords.Module
	now      func() time.Time
}

// ExternalSecurityDeps is what the Bearer-token handler is built from.
type ExternalSecurityDeps struct {
	// Ent looks the token up.
	Ent *ent.Client
	// Bus opens the transactions of audited writes made under the token (ADR 0022).
	Bus *events.Bus
	// ConsentRecords is the acceptance gate a Connection's User meets (ADR 0062).
	ConsentRecords *consentrecords.Module
	// Clock is the time a Connection's Two-factor requirement is checked against (nil means
	// time.Now).
	Clock func() time.Time
}

// NewExternalSecurityHandler builds the Bearer-token handler.
func NewExternalSecurityHandler(d ExternalSecurityDeps) *ExternalSecurityHandler {
	now := d.Clock
	if now == nil {
		now = time.Now
	}
	return &ExternalSecurityHandler{ent: d.Ent, bus: d.Bus, consents: d.ConsentRecords, now: now}
}

var _ externalapi.SecurityHandler = (*ExternalSecurityHandler)(nil)

// HandleBearerAuth authenticates the Bearer token (Authenticate) and then lets it call op only
// when it carries every scope the contract declares for op (ExternalOperationScopes, ADR 0061)
// and, for a Connection, when its User's current Role holds the Permission granting each of
// those scopes needs (accounts.GrantPermissions) and each Permission op itself declares
// (ExternalOperationPermissions). An operation absent from the scope table is refused to every
// token. A token that is missing, invalid, revoked or expired is ErrUnauthorized (401);
// a valid token without a scope of op is refused with 403, and a Connection whose User's Role
// lacks the Permission with 403 permission_required; neither counts against the address's
// failed-authentication budget.
func (h *ExternalSecurityHandler) HandleBearerAuth(ctx context.Context, op externalapi.OperationName, t externalapi.BearerAuth) (context.Context, error) {
	authed, err := h.Authenticate(ctx, t)
	if err != nil {
		return ctx, err
	}
	if err := permitted(op, GetTokenAuth(authed)); err != nil {
		return ctx, err
	}
	return authed, nil
}

// Authenticate resolves the Bearer token to its TokenAuth whatever it is used for, and applies
// the rate limits that need it (ADR 0025): a client address that spent its failed-authentication
// budget is refused before the token is looked at, a failure counts against it, and a success
// charges the Workspace's shared /api and /mcp budget. Rejections surface as
// *ratelimit.LimitedError. A Connection whose User has a required acceptance pending is refused
// with 403 acceptance_pending (ADR 0062), and one the Workspace's Two-factor requirement withholds
// with 403 second_factor_required (ADR 0020); the Workspace's API token meets neither. /mcp
// authenticates its requests with it; each tool call it dispatches to /api then meets the
// operation's scopes in HandleBearerAuth.
func (h *ExternalSecurityHandler) Authenticate(ctx context.Context, t externalapi.BearerAuth) (context.Context, error) {
	authed, err := guardedAuth(ctx,
		func() (context.Context, error) { return h.authenticate(ctx, t) },
		func(limits *ratelimit.Exchange, authed context.Context) error {
			return limits.ChargeWorkspace(GetTokenAuth(authed).WorkspaceID)
		})
	if err != nil {
		return ctx, err
	}
	return authed, nil
}

// permitted reports, as a refusal, whether the token may call op: the contract must declare op,
// the token must carry each of its scopes, and a Connection's User must hold, in their current
// Role, the Permission granting each scope needs and each Permission op declares, so a
// Connection does no more than its User may on /site: a demotion takes effect on the next request.
func permitted(op externalapi.OperationName, auth *TokenAuth) error {
	required, declared := ExternalOperationScopes[op]
	if !declared {
		return scopeRequiredError{op: op}
	}
	missing := lo.Reject(required, func(s apitokens.Scope, _ int) bool { return HasScope(auth, s) })
	if len(missing) > 0 {
		return scopeRequiredError{op: op, missing: missing}
	}
	if auth.Membership == nil {
		return nil
	}
	needed := lo.FilterMap(required, func(s apitokens.Scope, _ int) (accounts.Permission, bool) {
		p, ok := accounts.GrantPermissions[string(s)]
		return p, ok
	})
	for _, p := range append(needed, ExternalOperationPermissions[op]...) {
		if err := accounts.Require(auth.Membership.Role, p); err != nil {
			return err
		}
	}
	return nil
}

// scopeRequiredError refuses a valid API token an operation it lacks a scope of, or one the
// contract does not open to tokens at all. The server's error handler answers it through its
// Problem method.
type scopeRequiredError struct {
	op      externalapi.OperationName
	missing []apitokens.Scope
}

func (e scopeRequiredError) Error() string {
	if len(e.missing) == 0 {
		return "the operation " + e.op + " is not open to API tokens"
	}
	names := lo.Map(e.missing, func(s apitokens.Scope, _ int) string { return string(s) })
	if len(names) == 1 {
		return "the API token lacks the scope " + names[0]
	}
	return "the API token lacks the scopes " + strings.Join(names, ", ")
}

// Problem is the 403 a valid token without the scope is answered with.
func (e scopeRequiredError) Problem() (status int, code, detail string) {
	return http.StatusForbidden, "", e.Error()
}

// guardedAuth is the sequence both token surfaces share (ADR 0025): a client address
// that spent its failed-authentication budget is refused before the credential is
// looked at, an ErrUnauthorized counts against that budget, and a success is charged
// by charge (the Workspace's budget of the surface).
func guardedAuth[T any](ctx context.Context, authenticate func() (T, error), charge func(*ratelimit.Exchange, T) error) (T, error) {
	var zero T
	limits := ratelimit.FromContext(ctx)
	if err := limits.AuthBlocked(ctx); err != nil {
		return zero, err
	}
	authed, err := authenticate()
	if errors.Is(err, ErrUnauthorized) {
		if limited := limits.AuthFailed(ctx); limited != nil {
			return zero, limited
		}
		return zero, err
	}
	if err != nil {
		return zero, err
	}
	if err := charge(limits, authed); err != nil {
		return zero, err
	}
	return authed, nil
}

func (h *ExternalSecurityHandler) authenticate(ctx context.Context, t externalapi.BearerAuth) (context.Context, error) {
	parsed := credentials.ParseToken(t.Token)
	if parsed == nil {
		return ctx, ErrUnauthorized
	}

	token, err := h.ent.ApiToken.Query().
		Where(apitoken.Prefix(parsed.Prefix), apitoken.HasWorkspaceWith(accounts.Live())).
		WithMembership(func(q *ent.MembershipQuery) { q.WithUser().WithWorkspace() }).
		First(ctx)
	if ent.IsNotFound(err) {
		return ctx, ErrUnauthorized
	}
	if err != nil {
		// Real DB error (timeout, pool exhaustion, ...) must surface as 500,
		// not masquerade as a 401 for an otherwise-valid token.
		return ctx, err
	}
	if token.RevokedAt != nil {
		return ctx, ErrUnauthorized
	}
	if token.ExpiresAt != nil && token.ExpiresAt.Before(time.Now()) {
		return ctx, ErrUnauthorized
	}
	if !credentials.VerifySecret(parsed.Secret, token.SecretHash) {
		return ctx, ErrUnauthorized
	}

	actor := events.Actor{Kind: events.ActorAPIToken, ID: strconv.FormatInt(token.ID, 10), Name: token.Name}
	m := token.Edges.Membership
	if m != nil {
		// A Connection acts as its User, through its client, under the Workspace's Two-factor
		// requirement and the acceptance gate like the User's own session (ADR 0061, ADR 0020,
		// ADR 0062); a token carries no Second factor step. The Workspace's API token is no
		// User's and meets neither. The gate comes first, as on /site, so a User both gated and
		// withheld is refused alike on every surface.
		gated, err := h.consents.Gated(ctx, m.UserID)
		if err != nil {
			return ctx, err
		}
		if gated {
			return ctx, AcceptancePendingError{}
		}
		if secondfactor.Withheld(m, h.now(), false) {
			return ctx, SecondFactorRequiredError{}
		}
		actor = events.ConnectionActor(m.UserID, m.Edges.User.Name, token.Name)
	}

	auth := &TokenAuth{
		TokenID:     token.ID,
		WorkspaceID: token.WorkspaceID,
		Name:        token.Name,
		Scopes:      token.Scopes,
		Membership:  m,
		Scoped:      h.bus.Act(h.ent.Scoped(token.WorkspaceID), actor),
	}
	return WithTokenAuth(ctx, auth), nil
}

// SecondFactorRequiredError is secondfactor.ErrRequired as the problem every surface answers it
// with: a site session or a Connection of a User the Workspace's Two-factor requirement withholds
// (ADR 0020). Returned as an error, it is rendered by the server's error handler, which reads it
// through its Problem method.
type SecondFactorRequiredError struct{}

func (SecondFactorRequiredError) Error() string { return secondfactor.ErrRequired.Error() }
func (SecondFactorRequiredError) Unwrap() error { return secondfactor.ErrRequired }

// Problem is the 403 second_factor_required answer.
func (SecondFactorRequiredError) Problem() (status int, code, detail string) {
	return http.StatusForbidden, string(siteapi.ProblemCodeSecondFactorRequired), i18n.T("errors.second_factor_required", nil)
}

// AcceptancePendingError refuses, alike on every surface, a request on the own credential of a
// User with a required acceptance pending (consentrecords.Module.Gated, ADR 0062). Returned as an
// error, it is rendered by the server's error handler, which reads it through its Problem method.
type AcceptancePendingError struct{}

func (AcceptancePendingError) Error() string { return "auth: a required acceptance is pending" }

// Problem is the 403 acceptance_pending answer.
func (AcceptancePendingError) Problem() (status int, code, detail string) {
	return http.StatusForbidden, string(siteapi.ProblemCodeAcceptancePending), i18n.T("errors.acceptance_pending", nil)
}

// CollectAuth holds the workspace resolved from the per-workspace collect key.
type CollectAuth struct {
	WorkspaceID int64
	// Scoped is the client confined to WorkspaceID. This file is the collect
	// API's construction point of it (ADR 0017); read it with CollectScoped.
	Scoped *ent.Scoped
}

var collectAuthKey = struct{ name string }{"collectAuth"}

func WithCollectAuth(ctx context.Context, auth *CollectAuth) context.Context {
	return context.WithValue(ctx, collectAuthKey, auth)
}

func GetCollectAuth(ctx context.Context) *CollectAuth {
	v, _ := ctx.Value(collectAuthKey).(*CollectAuth)
	return v
}

// CollectScoped returns the Workspace-scoped client of the resolved collect key. It
// is nil on an unauthenticated context; the security handler runs before every
// collect operation, so handlers can rely on it.
func CollectScoped(ctx context.Context) *ent.Scoped {
	if a := GetCollectAuth(ctx); a != nil {
		return a.Scoped
	}
	return nil
}

var hostWorkspaceKey = struct{ name string }{"hostWorkspace"}

// WithHostWorkspace marks a request that arrived on a Workspace's own Tracking host (ADR 0059):
// the collect key must then be that Workspace's, so the host is no ingestion path for another key.
func WithHostWorkspace(ctx context.Context, workspaceID int64) context.Context {
	return context.WithValue(ctx, hostWorkspaceKey, workspaceID)
}

// CollectSecurityHandler implements collectapi.SecurityHandler: resolves the
// per-workspace collect write-key (x-collect-key) to its workspace.
type CollectSecurityHandler struct {
	ent *ent.Client
}

func NewCollectSecurityHandler(client *ent.Client) *CollectSecurityHandler {
	return &CollectSecurityHandler{ent: client}
}

var _ collectapi.SecurityHandler = (*CollectSecurityHandler)(nil)

// HandleApiKeyAuth authenticates the collect key and applies the rate limits that
// need it (ADR 0025): a client address that spent its failed-authentication budget
// is refused before the key is looked at, a wrong key counts against it, and a
// success charges the Workspace's /collect budget. Rejections surface as
// *ratelimit.LimitedError.
func (h *CollectSecurityHandler) HandleApiKeyAuth(ctx context.Context, _ collectapi.OperationName, t collectapi.ApiKeyAuth) (context.Context, error) {
	ws, err := guardedAuth(ctx,
		func() (*ent.Workspace, error) { return h.workspaceByKey(ctx, t.APIKey) },
		func(limits *ratelimit.Exchange, ws *ent.Workspace) error { return limits.ChargeCollect(ws.ID) })
	if err != nil {
		return ctx, err
	}
	return WithCollectAuth(ctx, &CollectAuth{WorkspaceID: ws.ID, Scoped: events.Ingest(h.ent.Scoped(ws.ID))}), nil
}

func (h *CollectSecurityHandler) workspaceByKey(ctx context.Context, key string) (*ent.Workspace, error) {
	if key == "" {
		return nil, ErrUnauthorized
	}
	ws, err := h.ent.Workspace.Query().Where(workspace.CollectKey(key), accounts.Live()).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	if host, onHost := ctx.Value(hostWorkspaceKey).(int64); onHost && host != ws.ID {
		return nil, ErrUnauthorized
	}
	return ws, nil
}

// SiteAuth holds the authenticated dashboard user resolved from the JWT cookie.
type SiteAuth struct {
	// Session is how the session began: its Method and, for a Sign-in provider login (ADR 0048),
	// the time of that login.
	Session

	UserID int64
	Email  string
}

var siteAuthKey = struct{ name string }{"siteAuth"}

func WithSiteAuth(ctx context.Context, auth *SiteAuth) context.Context {
	return context.WithValue(ctx, siteAuthKey, auth)
}

func GetSiteAuth(ctx context.Context) *SiteAuth {
	v, _ := ctx.Value(siteAuthKey).(*SiteAuth)
	return v
}

// SiteSecurityHandler implements siteapi.SecurityHandler: validates the JWT
// cookie issued by go-pkgz/auth and resolves the dashboard user from it.
//
// go-pkgz's Parse ignores an expired exp (its own middleware would refresh), so the
// expiry is enforced here against the injected clock; there is no refresh (ADR
// 0020). The User is looked up by the id the token carries, and the token's epoch
// must equal the User's session_epoch: a bump ends every earlier session.
type SiteSecurityHandler struct {
	verifier verifier
	ent      *ent.Client
	now      func() time.Time
}

// NewSiteSecurityHandler builds the cookie handler. now is the clock the token's
// expiry is checked against; nil means time.Now. A token verifies against jwtSecret and then
// against each of verifyWith, the secondary key of a JWT secret rotation.
func NewSiteSecurityHandler(jwtSecret string, client *ent.Client, now func() time.Time, verifyWith ...string) *SiteSecurityHandler {
	if now == nil {
		now = time.Now
	}
	return &SiteSecurityHandler{verifier: newVerifier(jwtSecret, verifyWith), ent: client, now: now}
}

var _ siteapi.SecurityHandler = (*SiteSecurityHandler)(nil)

func (h *SiteSecurityHandler) HandleApiKeyAuth(ctx context.Context, op siteapi.OperationName, t siteapi.ApiKeyAuth) (context.Context, error) {
	u, claims, err := h.verify(ctx, t.APIKey)
	if errors.Is(err, ErrUnauthorized) && op == siteapi.SitePublicInvitationsAcceptOperation {
		// The session is optional here: a stale cookie is the same as none, so an
		// invitee with no Verified User is not blocked by it.
		return ctx, ogenerrors.ErrSkipServerSecurity
	}
	if err != nil {
		return ctx, err
	}
	sess, _ := sessionOf(claims)
	return WithSiteAuth(ctx, &SiteAuth{UserID: u.ID, Email: u.Email, Session: sess}), nil
}

// Verify checks a raw session token (signature, expiry, User id and epoch) and
// returns the User it belongs to, or ErrUnauthorized.
func (h *SiteSecurityHandler) Verify(ctx context.Context, raw string) (*ent.User, error) {
	u, _, err := h.verify(ctx, raw)
	return u, err
}

// verify is Verify that also returns the token's claims.
func (h *SiteSecurityHandler) verify(ctx context.Context, raw string) (*ent.User, gptoken.Claims, error) {
	claims, err := h.verifier.parse(raw)
	if err != nil {
		return nil, claims, ErrUnauthorized
	}
	if claims.ExpiresAt == nil || !h.now().Before(claims.ExpiresAt.Time) {
		return nil, claims, ErrUnauthorized
	}
	id, epoch, ok := sessionUser(claims)
	if !ok {
		return nil, claims, ErrUnauthorized
	}
	if _, ok := sessionOf(claims); !ok {
		return nil, claims, ErrUnauthorized
	}

	u, err := h.ent.User.Get(ctx, id)
	if ent.IsNotFound(err) {
		return nil, claims, ErrUnauthorized
	}
	if err != nil {
		return nil, claims, err
	}
	if u.SessionEpoch != epoch {
		return nil, claims, ErrUnauthorized
	}
	return u, claims, nil
}
