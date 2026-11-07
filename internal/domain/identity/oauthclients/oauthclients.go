// Package oauthclients owns the OAuth clients of the authorization server for MCP connectors
// (ADR 0016, phase 2) and what belongs to a client (ADR 0102): the client a connector registers
// (RFC 7591) or names by its Client ID Metadata Document, the authorization code a signed-in
// User's consent records, and the refresh tokens of a grant. The consent screen is a SPA route;
// the signed-in user's decision arrives through the site API and is handled by [Module.Decide].
// Exchanging a code or a refresh token for a Connection (ADR 0061), the API token the grant
// issues, is the use case internal/usecase/connectionissuance; the protocol's endpoints are the
// driver internal/api/oauth.
//
// A client and its codes and refresh tokens belong to no Workspace before a code is exchanged
// (a code names the Workspace its consent chose), so the package works on the raw *ent.Client.
package oauthclients

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"net"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/oauthclient"
	"github.com/getsphericon/sphericon/ent/oauthcode"
	"github.com/getsphericon/sphericon/ent/oauthrefreshtoken"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

var (
	// ErrUnknownClient means the client_id was never registered.
	ErrUnknownClient = validation.NotFound("unknown client", i18n.ErrorsOauthClientUnknown)
	// ErrInvalidRequest means the authorization request is malformed or does not
	// match the client's registration (the message is safe to show).
	ErrInvalidRequest = validation.Refuse("invalid authorization request", i18n.ErrorsOauthRequestInvalid)
)

const (
	// codeTTL is how long an authorization code can be exchanged.
	codeTTL = 5 * time.Minute
	// refreshTokenTTL is how long a refresh token can be exchanged; every rotation starts it anew.
	refreshTokenTTL = 30 * 24 * time.Hour

	// MaxRedirectURIs bounds the redirect URIs a client registers.
	MaxRedirectURIs = 10
	// contract-copy: the OAuth dynamic client registration protocol is not in the contract
	maxClientName = 100
	defaultClient = "MCP client"

	// pkce verifiers and challenges are 43-128 unreserved characters (RFC 7636).
	minPKCELength = 43
	maxPKCELength = 128
)

// Module reads and writes the OAuth clients, their codes and their refresh tokens.
type Module struct {
	ent *ent.Client
	// fetcher downloads Client ID Metadata Documents; it must be an SSRF-hardened client.
	fetcher webhook.Doer
	// now is the Instance clock a code and a cached metadata document expire by.
	now func() time.Time
}

// New builds the module over the ent client; fetcher downloads Client ID Metadata Documents and
// must come from webhook.NewClient or NewClientAllowing; now is the Instance clock.
func New(client *ent.Client, fetcher webhook.Doer, now func() time.Time) *Module {
	return &Module{ent: client, fetcher: fetcher, now: now}
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

// Decision is the signed-in user's answer to an authorization request. Membership is the
// User's Membership of the Workspace the Connection will belong to, as the caller resolved it.
type Decision struct {
	ClientID      string
	RedirectURI   string
	State         string
	CodeChallenge string
	Scope         string
	Membership    *ent.Membership
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
// to: back to the client with a one-time code, or with access_denied.
func (s *Module) Decide(ctx context.Context, d Decision) (string, error) {
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
	if err := memberships.MayGrant(d.Membership.Role, granted); err != nil {
		return "", err
	}

	code := rand.Text()
	_, err = s.ent.OAuthCode.Create().
		SetCodeHash(hashSecret(code)).
		SetClientID(client.ID).
		SetWorkspaceID(d.Membership.WorkspaceID).
		SetMembershipID(d.Membership.ID).
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

// Register stores a client of dynamic client registration (RFC 7591) under a fresh client_id:
// named name, trimmed and bounded (a blank name is "MCP client"), and allowed exactly
// redirectURIs, which the caller has checked (ValidRedirectURI, MaxRedirectURIs).
func (s *Module) Register(ctx context.Context, name string, redirectURIs []string) (*ent.OAuthClient, error) {
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

// Code is the authorization code whose plaintext is code, with its client loaded; a NotFound
// error of ent when there is none. It reads the row whatever its state: the exchange claims it
// with ClaimCodeIn, which refuses a used or expired one.
func (s *Module) Code(ctx context.Context, code string) (*ent.OAuthCode, error) {
	return s.ent.OAuthCode.Query().
		Where(oauthcode.CodeHash(hashSecret(code))).
		WithClient().
		Only(ctx)
}

// ClaimCodeIn marks the authorization code id used at now on the transaction tx, so it is
// exchanged once: false when it is already used or expired, or a concurrent request claimed it
// first.
func ClaimCodeIn(ctx context.Context, tx *ent.Client, id int64, now time.Time) (bool, error) {
	claimed, err := tx.OAuthCode.Update().
		Where(oauthcode.ID(id), oauthcode.UsedAtIsNil(), oauthcode.ExpiresAtGT(now)).
		SetUsedAt(now).
		Save(ctx)
	return claimed > 0, err
}

// DeleteCodesIn deletes every authorization code that names the Workspace, on the transaction
// tx: the last step of erasing a Workspace (ADR 0043), whose codes are not Workspace-scoped rows.
func DeleteCodesIn(ctx context.Context, tx *ent.Client, workspaceID int64) error {
	_, err := tx.OAuthCode.Delete().Where(oauthcode.WorkspaceID(workspaceID)).Exec(ctx)
	return err
}

// RefreshToken is the refresh token whose plaintext is presented, with its client and the API
// token of its grant loaded; a NotFound error of ent when there is none.
func (s *Module) RefreshToken(ctx context.Context, presented string) (*ent.OAuthRefreshToken, error) {
	return s.ent.OAuthRefreshToken.Query().
		Where(oauthrefreshtoken.TokenHash(hashSecret(presented))).
		WithClient().
		WithAPIToken().
		Only(ctx)
}

// ClaimRefreshTokenIn marks the refresh token id used at now on the transaction tx, so it is
// exchanged once: false when it is already used or expired, or a concurrent request claimed it
// first.
func ClaimRefreshTokenIn(ctx context.Context, tx *ent.Client, id int64, now time.Time) (bool, error) {
	claimed, err := tx.OAuthRefreshToken.Update().
		Where(oauthrefreshtoken.ID(id), oauthrefreshtoken.UsedAtIsNil(), oauthrefreshtoken.ExpiresAtGT(now)).
		SetUsedAt(now).
		Save(ctx)
	return claimed > 0, err
}

// PruneRefreshTokensIn deletes the refresh tokens of the grant of the API token apiTokenID that
// expired before now, on the transaction tx.
func PruneRefreshTokensIn(ctx context.Context, tx *ent.Client, apiTokenID int64, now time.Time) error {
	_, err := tx.OAuthRefreshToken.Delete().
		Where(oauthrefreshtoken.APITokenID(apiTokenID), oauthrefreshtoken.ExpiresAtLT(now)).
		Exec(ctx)
	return err
}

// NewRefreshTokenIn stores a fresh refresh token of the client clientRowID for the grant of the
// API token apiTokenID on the transaction tx, valid from now, and returns its plaintext. Only
// the exchange of a code or a refresh token calls it (internal/usecase/connectionissuance),
// after it has checked and claimed what was presented.
func NewRefreshTokenIn(ctx context.Context, tx *ent.Client, clientRowID, apiTokenID int64, now time.Time) (string, error) {
	plain := rand.Text()
	_, err := tx.OAuthRefreshToken.Create().
		SetTokenHash(hashSecret(plain)).
		SetClientID(clientRowID).
		SetAPITokenID(apiTokenID).
		SetExpiresAt(now.Add(refreshTokenTTL)).
		Save(ctx)
	return plain, err
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
