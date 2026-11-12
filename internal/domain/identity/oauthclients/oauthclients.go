// Package oauthclients owns the OAuth clients of the authorization server for MCP connectors
// (ADR 0016, phase 2) and what belongs to a client (ADR 0102): the client a connector registers
// (RFC 7591) or names by its Client ID Metadata Document, and the authorization code a signed-in
// User's consent records. The consent screen is a SPA route; the signed-in user's decision arrives
// through the site API and is handled by [Module.Decide], which admits the User to the Workspace
// the decision names first, as every operation of a Workspace is (admission, ADR 0061). A code is claimed by [ClaimCodeIn] only
// after its own check of the client, the redirect URI and the PKCE verifier; exchanging it for a
// Connection (ADR 0061), the API token the grant issues, and the Connection's refresh tokens are
// internal/domain/identity/apitokens's (ADR 0110); the protocol's endpoints are the driver
// internal/api/oauth.
//
// A client and its codes belong to no Workspace before a code is exchanged (a code names the
// Workspace its consent chose), so the package works on the raw *ent.Client.
package oauthclients

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"maps"
	"net"
	"net/url"
	"slices"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/oauthclient"
	"github.com/getsphericon/sphericon/ent/oauthcode"
	"github.com/getsphericon/sphericon/internal/domain/identity/admission"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// The unique constraints of the tables this package writes (ADR 0106).
var _ = db.Declare(
	db.Fault("oauth_clients_client_id_key", "Register draws the client_id at random, and a client of a metadata document is written with ON CONFLICT on its URL"),
	db.Fault("oauth_codes_code_hash_key", "the code is drawn at random and stored as its hash"),
)

var (
	// ErrUnknownClient means the client_id was never registered.
	ErrUnknownClient = validation.NotFound("unknown client", i18n.ErrorsOauthClientUnknown)
	// ErrInvalidRequest means the authorization request is malformed or does not
	// match the client's registration (the message is safe to show).
	ErrInvalidRequest = validation.Refuse("invalid authorization request", i18n.ErrorsOauthRequestInvalid)
	// ErrInvalidGrant is a code or a refresh token that names no grant the token request may use:
	// unknown, expired, already used, or issued to another client or redirect URI (RFC 6749 section
	// 5.2, invalid_grant). A refresh token's refusal (internal/domain/identity/apitokens) is one too.
	ErrInvalidGrant = validation.UnknownReference("oauthclients: invalid grant", i18n.ErrorsOauthGrantInvalid)
)

var errUnknownCode = InvalidGrant("unknown or expired code")

// InvalidGrant is ErrInvalidGrant with what was wrong with the grant.
func InvalidGrant(description string) error { return ErrInvalidGrant.With(description) }

const (
	// codeTTL is how long an authorization code can be exchanged.
	codeTTL = 5 * time.Minute

	// MaxRedirectURIs bounds the redirect URIs a client registers.
	MaxRedirectURIs = 10
	// maxClientName bounds a registered client's name; the registration protocol is not in the
	// contract, so the bound is this module's own.
	maxClientName = 100
	defaultClient = "MCP client"

	// pkce verifiers and challenges are 43-128 unreserved characters (RFC 7636).
	minPKCELength = 43
	maxPKCELength = 128
)

// Module reads and writes the OAuth clients and their codes.
type Module struct {
	ent *ent.Client
	// fetcher downloads Client ID Metadata Documents; it must be an SSRF-hardened client.
	fetcher webhook.Doer
	// admit admits the User deciding to the Workspace their decision names.
	admit Admitter
	// now is the Instance clock a code and a cached metadata document expire by.
	now func() time.Time
}

// Admitter admits a request made on a User's session to a Workspace (admission.Module.Admit).
type Admitter interface {
	Admit(ctx context.Context, r admission.Request) (admission.Admitted, error)
}

// New builds the module over the ent client; fetcher downloads Client ID Metadata Documents and
// must come from webhook.NewClient or NewClientAllowing; admit admits the User whose decision
// Decide records; now is the Instance clock.
func New(client *ent.Client, fetcher webhook.Doer, admit Admitter, now func() time.Time) *Module {
	return &Module{ent: client, fetcher: fetcher, admit: admit, now: now}
}

// Request is a validated authorization request, ready for the consent screen.
type Request struct {
	ClientName string
	// ClientHost is the host of a metadata client's client_id URL, empty for a registered client.
	ClientHost string
	// LocalRedirect is set when the redirect URI is on a loopback host, where any program
	// on the user's machine can be listening.
	LocalRedirect bool
	RedirectURI   string
	// Scopes are granted by default; SendScopes and SetupScopes only on an explicit opt-in
	// each (a box of its own on the consent screen).
	Scopes      []string
	SendScopes  []string
	SetupScopes []string
	// RequiredPermissions names, for each requested scope that needs one, the Permission a User
	// must hold to grant it (memberships.GrantPermissions, ADR 0061).
	RequiredPermissions map[string]memberships.Permission
}

// Decision is the signed-in user's answer to an authorization request. Admission is the request
// made on the User's session, naming the Workspace the Connection will belong to.
type Decision struct {
	ClientID      string
	RedirectURI   string
	State         string
	CodeChallenge string
	Scope         string
	Admission     admission.Request
	Approve       bool
	AllowSend     bool
	AllowSetup    bool
}

// Describe validates the client and redirect URI of an authorization request and
// reports what it asks for.
func (s *Module) Describe(ctx context.Context, clientID, redirectURI, scope string) (*Request, error) {
	client, err := s.Client(ctx, clientID, redirectURI)
	if err != nil {
		return nil, err
	}
	granted, send, setup := requestedScopes(scope)
	if strings.TrimSpace(scope) != "" && len(granted)+len(send)+len(setup) == 0 {
		return nil, ErrInvalidRequest.With("no supported scope requested")
	}
	return &Request{
		ClientName: client.Name, ClientHost: ClientHost(clientID), LocalRedirect: IsLoopback(redirectURI),
		RedirectURI: redirectURI, Scopes: granted, SendScopes: send, SetupScopes: setup,
		RequiredPermissions: memberships.GrantRequires(slices.Concat(granted, send, setup)),
	}, nil
}

// Decide records the user's decision and returns the URL to send their browser
// to: back to the client with a one-time code, or with access_denied. The User is admitted to the
// Workspace the decision names before anything else, as Access admits the Membership of any
// operation of a Workspace: workspaces.ErrNotFound for a Workspace the User is no member of (or
// none named), then the refusals of the admission. Any member may approve, but only the scopes
// their Role may grant (memberships.MayGrant).
func (s *Module) Decide(ctx context.Context, d Decision) (string, error) {
	if d.Admission.Workspace == "" {
		return "", workspaces.ErrNotFound
	}
	admitted, err := s.admit.Admit(ctx, d.Admission)
	if err != nil {
		return "", err
	}
	member := admitted.Membership
	client, err := s.Client(ctx, d.ClientID, d.RedirectURI)
	if err != nil {
		return "", err
	}
	if !d.Approve {
		return RedirectWith(d.RedirectURI, url.Values{"error": {"access_denied"}}, d.State)
	}
	if !ValidPKCE(d.CodeChallenge) {
		return "", ErrInvalidRequest.With("code_challenge must be a PKCE S256 challenge")
	}
	granted, send, setup := requestedScopes(d.Scope)
	if strings.TrimSpace(d.Scope) != "" && len(granted)+len(send)+len(setup) == 0 {
		return "", ErrInvalidRequest.With("no supported scope requested")
	}
	if d.AllowSend {
		granted = append(granted, send...)
	}
	if d.AllowSetup {
		granted = append(granted, setup...)
	}
	// Any member may approve, but only the scopes their Role may grant (ADR 0061): the consent
	// screen offers no other, and the first one beyond the Role refuses the whole decision.
	if err := memberships.MayGrant(member.Role, granted); err != nil {
		return "", err
	}

	code := rand.Text()
	_, err = s.ent.OAuthCode.Create().
		SetCodeHash(hashSecret(code)).
		SetClientID(client.ID).
		SetWorkspaceID(member.WorkspaceID).
		SetMembershipID(member.ID).
		SetRedirectURI(d.RedirectURI).
		SetCodeChallenge(d.CodeChallenge).
		SetScopes(granted).
		SetExpiresAt(s.now().Add(codeTTL)).
		Save(ctx)
	if err != nil {
		return "", err
	}
	return RedirectWith(d.RedirectURI, url.Values{"code": {code}}, d.State)
}

// Client loads a registered client, or the client a metadata client_id names, and checks
// redirectURI against its exact registered list (never a prefix or pattern match).
// ErrUnknownClient for a client_id never registered, ErrInvalidRequest for a redirect URI it
// does not list.
func (s *Module) Client(ctx context.Context, clientID, redirectURI string) (*ent.OAuthClient, error) {
	var c *ent.OAuthClient
	var err error
	if IsMetadataClientID(clientID) {
		c, err = s.metadataClient(ctx, clientID)
	} else {
		c, err = s.ent.OAuthClient.Query().Where(oauthclient.ClientID(clientID)).Only(ctx)
	}
	if ent.IsNotFound(err) {
		return nil, ErrUnknownClient
	}
	if err != nil {
		return nil, err
	}
	if !slices.Contains(c.RedirectUris, redirectURI) {
		return nil, ErrInvalidRequest.With("redirect_uri is not registered for this client")
	}
	return c, nil
}

// RedirectWith is redirectURI with params and, when set, state added to its query: where the
// browser goes back to the client.
func RedirectWith(redirectURI string, params url.Values, state string) (string, error) {
	u, err := url.Parse(redirectURI)
	if err != nil {
		return "", ErrInvalidRequest.With("bad redirect_uri")
	}
	q := u.Query()
	maps.Copy(q, params)
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// ValidPKCE reports whether s has the shape of a PKCE verifier or S256 challenge: 43 to 128
// unreserved characters (RFC 7636).
func ValidPKCE(s string) bool {
	if len(s) < minPKCELength || len(s) > maxPKCELength {
		return false
	}
	for _, r := range s {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-._~", r)
		if !ok {
			return false
		}
	}
	return true
}

func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// The refusals of a registration's redirect URIs (RFC 7591 invalid_redirect_uri).

// The OAuth words of the one client kind this server registers (ADR 0016): a public client, which
// authenticates at the token endpoint with nothing (AuthMethodNone), takes the authorization code
// (ResponseTypeCode) and proves it with PKCE's S256 (PKCEMethodS256).
const (
	AuthMethodNone   = "none"
	ResponseTypeCode = "code"
	PKCEMethodS256   = "S256"
)

// fieldRedirectURIs is the client metadata field of the redirect URIs (RFC 7591): the dynamic
// registration is OAuth's endpoint, outside the contract, so its field names are the RFC's.
const fieldRedirectURIs = "redirect_uris"

var (
	// ErrRedirectURIsCount is a registration with no redirect URI or more than MaxRedirectURIs.
	ErrRedirectURIsCount = validation.RefuseField(fieldRedirectURIs, "oauthclients: wrong number of redirect URIs",
		i18n.ErrorsOauthRedirectUrisCount)
	// ErrRedirectURIInvalid is a redirect URI ValidRedirectURI refuses.
	ErrRedirectURIInvalid = validation.RefuseField(fieldRedirectURIs, "oauthclients: invalid redirect URI",
		i18n.ErrorsOauthRedirectUriInvalid)
)

// Register stores a client of dynamic client registration (RFC 7591) under a fresh client_id:
// named name, trimmed and bounded (a blank name is "MCP client"), and allowed exactly
// redirectURIs: one to MaxRedirectURIs of them, each one ValidRedirectURI accepts
// (ErrRedirectURIsCount, ErrRedirectURIInvalid).
func (s *Module) Register(ctx context.Context, name string, redirectURIs []string) (*ent.OAuthClient, error) {
	if len(redirectURIs) == 0 || len(redirectURIs) > MaxRedirectURIs {
		return nil, ErrRedirectURIsCount.Args(map[string]any{"Max": MaxRedirectURIs})
	}
	for _, u := range redirectURIs {
		if !ValidRedirectURI(u) {
			return nil, ErrRedirectURIInvalid
		}
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = defaultClient
	}
	return s.ent.OAuthClient.Create().
		SetClientID(rand.Text()).
		SetName(truncate(name, maxClientName)).
		SetRedirectUris(redirectURIs).
		Save(ctx)
}

// CodeClaim is a token request with an authorization code: the code, the client it was issued
// to, the redirect URI of the authorization request and the PKCE verifier.
type CodeClaim struct {
	Code        string
	ClientID    string
	RedirectURI string
	Verifier    string
}

// ClaimCodeIn spends the authorization code of the claim c at now on the transaction tx and returns
// it with its client loaded: the grant the consent recorded, whose Workspace it names. It checks
// the request first (the client and the redirect URI of the authorization request, and the PKCE
// verifier against the code's challenge), so a refused request leaves the code to the right one;
// then it marks the code used, so it is exchanged once. ErrInvalidGrant for an unknown, expired or
// used code, one a concurrent request claimed first, or a request it was not issued for.
func ClaimCodeIn(ctx context.Context, tx *ent.Client, c CodeClaim, now time.Time) (*ent.OAuthCode, error) {
	grant, err := tx.OAuthCode.Query().
		Where(oauthcode.CodeHash(hashSecret(c.Code))).
		WithClient().
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, errUnknownCode
	}
	if err != nil {
		return nil, err
	}

	switch {
	case grant.Edges.Client.ClientID != c.ClientID, grant.RedirectURI != c.RedirectURI:
		return nil, InvalidGrant("client or redirect_uri does not match the authorization request")
	case !ValidPKCE(c.Verifier),
		subtle.ConstantTimeCompare([]byte(oauth2.S256ChallengeFromVerifier(c.Verifier)), []byte(grant.CodeChallenge)) != 1:
		return nil, InvalidGrant("code_verifier does not match")
	}

	// Single use: only the request that flips used_at wins.
	err = lifecycle.Latch(ctx, func(ctx context.Context) (bool, error) {
		claimed, err := tx.OAuthCode.Update().
			Where(oauthcode.ID(grant.ID), oauthcode.UsedAtIsNil(), oauthcode.ExpiresAtGT(now)).
			SetUsedAt(now).
			Save(ctx)
		return claimed > 0, err
	})
	if errors.Is(err, lifecycle.ErrInvalidTransition) {
		return nil, errUnknownCode
	}
	if err != nil {
		return nil, err
	}
	return grant, nil
}

// ValidRedirectURI accepts https URLs and http on a loopback host, with no fragment (RFC 6749
// 3.1.2, OAuth 2.1).
func ValidRedirectURI(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Fragment != "" || u.User != nil {
		return false
	}
	switch u.Scheme {
	case "https":
		return true
	case "http":
		return IsLoopback(raw)
	}
	return false
}

// IsLoopback reports whether a redirect URI points at this machine.
func IsLoopback(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	return host == "localhost" || ip != nil && ip.IsLoopback()
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
