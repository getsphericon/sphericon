// Package oauthserver is the OAuth 2.1 authorization server for MCP connectors
// (ADR 0016, phase 2): discovery documents (RFC 9728, RFC 8414), dynamic client
// registration (RFC 7591), the PKCE authorization-code flow and the token
// endpoint. The consent screen is a SPA route; the signed-in user's decision
// arrives through the site API and is handled by [Service.Decide]. The token
// endpoint issues a Connection (ADR 0061): an API token tied to the approving
// User's Membership, which /mcp and /api authenticate like a hand-made token and
// then bound by that User's current Role.
package oauthserver

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/oauthclient"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/validation"
	"github.com/getsphericon/sphericon/internal/webhook"
)

var (
	// ErrUnknownClient means the client_id was never registered.
	ErrUnknownClient = validation.NotFound("unknown client", "errors.oauth_client_unknown")
	// ErrInvalidRequest means the authorization request is malformed or does not
	// match the client's registration (the message is safe to show).
	ErrInvalidRequest = validation.Refuse("invalid authorization request", "errors.oauth_request_invalid")
)

const (
	// codeTTL is how long an authorization code can be exchanged.
	codeTTL = 5 * time.Minute

	// pkce verifiers and challenges are 43-128 unreserved characters (RFC 7636).
	minPKCELength = 43
	maxPKCELength = 128
)

// Service holds the authorization server's state operations.
type Service struct {
	ent *ent.Client
	// fetcher downloads Client ID Metadata Documents; it must be an SSRF-hardened client.
	fetcher webhook.Doer
}

// NewService builds the service over the ent client; fetcher downloads Client ID Metadata
// Documents and must come from webhook.NewClient or NewClientAllowing.
func NewService(client *ent.Client, fetcher webhook.Doer) *Service {
	return &Service{ent: client, fetcher: fetcher}
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
	// must hold to grant it (accounts.GrantPermissions, ADR 0061).
	RequiredPermissions map[string]accounts.Permission
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
func (s *Service) Describe(ctx context.Context, clientID, redirectURI, scope string) (*Request, error) {
	client, err := s.client(ctx, clientID, redirectURI)
	if err != nil {
		return nil, err
	}
	granted, send, setup := requestedScopes(scope)
	if strings.TrimSpace(scope) != "" && len(granted)+len(send)+len(setup) == 0 {
		return nil, ErrInvalidRequest.With("no supported scope requested")
	}
	return &Request{
		ClientName: client.Name, ClientHost: ClientHost(clientID), LocalRedirect: isLoopback(redirectURI),
		RedirectURI: redirectURI, Scopes: granted, SendScopes: send, SetupScopes: setup,
		RequiredPermissions: accounts.GrantRequires(slices.Concat(granted, send, setup)),
	}, nil
}

// Decide records the user's decision and returns the URL to send their browser
// to: back to the client with a one-time code, or with access_denied.
func (s *Service) Decide(ctx context.Context, d Decision) (string, error) {
	client, err := s.client(ctx, d.ClientID, d.RedirectURI)
	if err != nil {
		return "", err
	}
	if !d.Approve {
		return redirectWith(d.RedirectURI, url.Values{"error": {"access_denied"}}, d.State)
	}
	if !validPKCE(d.CodeChallenge) {
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
	if err := accounts.MayGrant(d.Membership.Role, granted); err != nil {
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
		SetExpiresAt(time.Now().Add(codeTTL)).
		Save(ctx)
	if err != nil {
		return "", err
	}
	return redirectWith(d.RedirectURI, url.Values{"code": {code}}, d.State)
}

// client loads a registered client and checks redirectURI against its exact
// registered list (never a prefix or pattern match).
func (s *Service) client(ctx context.Context, clientID, redirectURI string) (*ent.OAuthClient, error) {
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

func redirectWith(redirectURI string, params url.Values, state string) (string, error) {
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

func validPKCE(s string) bool {
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
