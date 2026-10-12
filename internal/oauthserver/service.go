// Package oauthserver is the OAuth 2.1 authorization server for MCP connectors
// (ADR 0016, phase 2): discovery documents (RFC 9728, RFC 8414), dynamic client
// registration (RFC 7591), the PKCE authorization-code flow and the token
// endpoint. The consent screen is a SPA route; the signed-in user's decision
// arrives through the site API and is handled by [Service.Decide]. The token
// endpoint issues ordinary scoped API tokens, so /mcp and /api authenticate
// them exactly like a hand-made token.
package oauthserver

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/oauthclient"
)

var (
	// ErrUnknownClient means the client_id was never registered.
	ErrUnknownClient = errors.New("unknown client")
	// ErrInvalidRequest means the authorization request is malformed or does not
	// match the client's registration (the message is safe to show).
	ErrInvalidRequest = errors.New("invalid authorization request")
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
}

// NewService builds the service over the ent client.
func NewService(client *ent.Client) *Service { return &Service{ent: client} }

// Request is a validated authorization request, ready for the consent screen.
type Request struct {
	ClientName  string
	RedirectURI string
	// Scopes are granted by default; SendScopes only on explicit opt-in.
	Scopes     []string
	SendScopes []string
}

// Decision is the signed-in user's answer to an authorization request. The
// caller has already verified the user belongs to WorkspaceID.
type Decision struct {
	ClientID      string
	RedirectURI   string
	State         string
	CodeChallenge string
	Scope         string
	WorkspaceID   int64
	Approve       bool
	AllowSend     bool
}

// Describe validates the client and redirect URI of an authorization request and
// reports what it asks for.
func (s *Service) Describe(ctx context.Context, clientID, redirectURI, scope string) (*Request, error) {
	client, err := s.client(ctx, clientID, redirectURI)
	if err != nil {
		return nil, err
	}
	granted, send := requestedScopes(scope)
	if strings.TrimSpace(scope) != "" && len(granted) == 0 && len(send) == 0 {
		return nil, fmt.Errorf("%w: no supported scope requested", ErrInvalidRequest)
	}
	return &Request{ClientName: client.Name, RedirectURI: redirectURI, Scopes: granted, SendScopes: send}, nil
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
		return "", fmt.Errorf("%w: code_challenge must be a PKCE S256 challenge", ErrInvalidRequest)
	}
	granted, send := requestedScopes(d.Scope)
	if strings.TrimSpace(d.Scope) != "" && len(granted) == 0 && len(send) == 0 {
		return "", fmt.Errorf("%w: no supported scope requested", ErrInvalidRequest)
	}
	if d.AllowSend {
		granted = append(granted, send...)
	}

	code := rand.Text()
	_, err = s.ent.OAuthCode.Create().
		SetCodeHash(hashCode(code)).
		SetClientID(client.ID).
		SetWorkspaceID(d.WorkspaceID).
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
	c, err := s.ent.OAuthClient.Query().Where(oauthclient.ClientID(clientID)).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrUnknownClient
	}
	if err != nil {
		return nil, err
	}
	if !slices.Contains(c.RedirectUris, redirectURI) {
		return nil, fmt.Errorf("%w: redirect_uri is not registered for this client", ErrInvalidRequest)
	}
	return c, nil
}

func redirectWith(redirectURI string, params url.Values, state string) (string, error) {
	u, err := url.Parse(redirectURI)
	if err != nil {
		return "", fmt.Errorf("%w: bad redirect_uri", ErrInvalidRequest)
	}
	q := u.Query()
	for k, v := range params {
		q[k] = v
	}
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

func hashCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}
