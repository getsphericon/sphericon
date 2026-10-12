package oauthserver

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/oauthcode"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"
)

// Paths served by the Go binary. The consent screen is the SPA route ConsentPath.
const (
	protectedResourcePath = "/.well-known/oauth-protected-resource"
	authServerPath        = "/.well-known/oauth-authorization-server"
	registerPath          = "/oauth/register"
	authorizePath         = "/oauth/authorize"
	tokenPath             = "/oauth/token"

	// ConsentPath is the SPA route that renders the consent screen.
	ConsentPath = "/oauth/consent"

	mcpPath = "/mcp"

	maxRedirectURIs = 10
	maxClientName   = 100
	defaultClient   = "MCP client"
)

// ResourceURL is the canonical URI of the protected MCP resource, served from
// the MCP origin (the app origin unless MCP_URL gives MCP its own host).
func ResourceURL(mcpOrigin string) string { return strings.TrimSuffix(mcpOrigin, "/") + mcpPath }

// ResourceMetadataURL is where /mcp points unauthenticated clients (RFC 9728),
// on the MCP origin.
func ResourceMetadataURL(mcpOrigin string) string {
	return strings.TrimSuffix(mcpOrigin, "/") + protectedResourcePath + mcpPath
}

// Server mounts the OAuth protocol endpoints.
type Server struct {
	*Service
	issuer   string
	resource string
}

// New builds the server; appURL is the public origin and the OAuth issuer, and
// mcpURL the origin of the MCP resource (the same as appURL on a single host).
func New(client *ent.Client, appURL, mcpURL string) *Server {
	return &Server{Service: NewService(client), issuer: strings.TrimSuffix(appURL, "/"), resource: ResourceURL(mcpURL)}
}

// Mount registers discovery, registration, authorize and token endpoints.
func (s *Server) Mount(mux *http.ServeMux) {
	prm := auth.ProtectedResourceMetadataHandler(&oauthex.ProtectedResourceMetadata{
		Resource:               s.resource,
		AuthorizationServers:   []string{s.issuer},
		ScopesSupported:        SupportedScopes(),
		BearerMethodsSupported: []string{"header"},
		ResourceName:           "sphericon",
	})
	// RFC 9728 inserts the resource path after the well-known prefix; the bare
	// path is served too for clients that probe the origin.
	mux.Handle("GET "+protectedResourcePath, prm)
	mux.Handle("GET "+protectedResourcePath+mcpPath, prm)
	mux.HandleFunc("GET "+authServerPath, s.authServerMetadata)
	mux.HandleFunc("POST "+registerPath, s.register)
	mux.HandleFunc("GET "+authorizePath, s.authorize)
	mux.HandleFunc("POST "+tokenPath, s.token)
}

func (s *Server) authServerMetadata(w http.ResponseWriter, _ *http.Request) {
	meta := struct {
		oauthex.AuthServerMeta
		CodeChallengeMethodsSupported []string `json:"code_challenge_methods_supported"`
	}{
		AuthServerMeta: oauthex.AuthServerMeta{
			Issuer:                            s.issuer,
			AuthorizationEndpoint:             s.issuer + authorizePath,
			TokenEndpoint:                     s.issuer + tokenPath,
			RegistrationEndpoint:              s.issuer + registerPath,
			ScopesSupported:                   SupportedScopes(),
			ResponseTypesSupported:            []string{"code"},
			GrantTypesSupported:               []string{"authorization_code"},
			TokenEndpointAuthMethodsSupported: []string{"none"},
		},
		CodeChallengeMethodsSupported: []string{"S256"},
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	writeJSON(w, http.StatusOK, meta)
}

// register implements RFC 7591 dynamic client registration for public clients.
func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var meta oauthex.ClientRegistrationMetadata
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&meta); err != nil {
		registrationError(w, "invalid_client_metadata", "body must be a JSON object")
		return
	}
	if len(meta.RedirectURIs) == 0 || len(meta.RedirectURIs) > maxRedirectURIs {
		registrationError(w, "invalid_redirect_uri", "provide between 1 and 10 redirect_uris")
		return
	}
	for _, u := range meta.RedirectURIs {
		if !validRedirectURI(u) {
			registrationError(w, "invalid_redirect_uri", "redirect_uris must be https URLs (or http on a loopback host) without a fragment")
			return
		}
	}
	if m := meta.TokenEndpointAuthMethod; m != "" && m != "none" {
		registrationError(w, "invalid_client_metadata", "only token_endpoint_auth_method none is supported (public clients with PKCE)")
		return
	}
	if len(meta.GrantTypes) > 0 && !slices.Contains(meta.GrantTypes, "authorization_code") {
		registrationError(w, "invalid_client_metadata", "grant_types must include authorization_code")
		return
	}

	name := strings.TrimSpace(meta.ClientName)
	if name == "" {
		name = defaultClient
	}
	name = truncate(name, maxClientName)
	clientID := rand.Text()
	row, err := s.ent.OAuthClient.Create().
		SetClientID(clientID).
		SetName(name).
		SetRedirectUris(meta.RedirectURIs).
		Save(r.Context())
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "")
		return
	}

	resp := oauthex.ClientRegistrationResponse{
		ClientRegistrationMetadata: oauthex.ClientRegistrationMetadata{
			RedirectURIs:            row.RedirectUris,
			TokenEndpointAuthMethod: "none",
			GrantTypes:              []string{"authorization_code"},
			ResponseTypes:           []string{"code"},
			ClientName:              row.Name,
		},
		ClientID:         row.ClientID,
		ClientIDIssuedAt: row.CreatedAt,
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	writeJSON(w, http.StatusCreated, &resp)
}

// authorize validates the request and hands the browser to the SPA consent
// screen, which asks the signed-in user and posts the decision to the site API.
func (s *Server) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	redirectURI := q.Get("redirect_uri")

	// Until the client and redirect URI check out, errors must not redirect.
	if _, err := s.client(r.Context(), q.Get("client_id"), redirectURI); err != nil {
		if errors.Is(err, ErrUnknownClient) || errors.Is(err, ErrInvalidRequest) {
			writeOAuthError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "")
		return
	}
	if kind, msg := checkAuthorizeParams(q, s.resource); msg != "" {
		target, err := redirectWith(redirectURI, url.Values{"error": {kind}, "error_description": {msg}}, q.Get("state"))
		if err != nil {
			writeOAuthError(w, http.StatusBadRequest, kind, msg)
			return
		}
		http.Redirect(w, r, target, http.StatusFound)
		return
	}
	http.Redirect(w, r, s.issuer+ConsentPath+"?"+q.Encode(), http.StatusFound)
}

// checkAuthorizeParams returns the OAuth error code and description for an
// invalid authorization request. A resource indicator (RFC 8707), when sent,
// must name the MCP resource this server protects; an absent one is accepted.
func checkAuthorizeParams(q url.Values, resource string) (kind, msg string) {
	switch {
	case q.Get("response_type") != "code":
		return "invalid_request", "response_type must be code"
	case q.Get("code_challenge_method") != "S256" || !validPKCE(q.Get("code_challenge")):
		return "invalid_request", "PKCE with code_challenge_method S256 is required"
	case q.Get("resource") != "" && q.Get("resource") != resource:
		return "invalid_target", "unknown resource"
	}
	return "", ""
}

// token exchanges an authorization code (with its PKCE verifier) for an ordinary
// scoped API token.
func (s *Server) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "body must be form encoded")
		return
	}
	if r.PostForm.Get("grant_type") != "authorization_code" {
		writeOAuthError(w, http.StatusBadRequest, "unsupported_grant_type", "only authorization_code is supported")
		return
	}
	if res := r.PostForm.Get("resource"); res != "" && res != s.resource {
		writeOAuthError(w, http.StatusBadRequest, "invalid_target", "unknown resource")
		return
	}
	ctx := r.Context()

	grant, err := s.ent.OAuthCode.Query().
		Where(oauthcode.CodeHash(hashCode(r.PostForm.Get("code")))).
		WithClient().
		Only(ctx)
	if ent.IsNotFound(err) {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "unknown or expired code")
		return
	}
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "")
		return
	}

	verifier := r.PostForm.Get("code_verifier")
	switch {
	case grant.Edges.Client.ClientID != r.PostForm.Get("client_id"),
		grant.RedirectURI != r.PostForm.Get("redirect_uri"):
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "client or redirect_uri does not match the authorization request")
		return
	case !validPKCE(verifier),
		subtle.ConstantTimeCompare([]byte(oauth2.S256ChallengeFromVerifier(verifier)), []byte(grant.CodeChallenge)) != 1:
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "code_verifier does not match")
		return
	}

	// Single use: only the request that flips used_at wins.
	now := time.Now()
	claimed, err := s.ent.OAuthCode.Update().
		Where(oauthcode.ID(grant.ID), oauthcode.UsedAtIsNil(), oauthcode.ExpiresAtGT(now)).
		SetUsedAt(now).
		Save(ctx)
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "")
		return
	}
	if claimed == 0 {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "unknown or expired code")
		return
	}

	prefix, err := credentials.GenerateTokenPrefix()
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "")
		return
	}
	secret, err := credentials.GenerateTokenSecret()
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "")
		return
	}
	hash, err := credentials.HashTokenSecret(secret)
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "")
		return
	}
	if _, err := s.ent.ApiToken.Create().
		SetName(grant.Edges.Client.Name + " (MCP)").
		SetPrefix(prefix).
		SetSecretHash(hash).
		SetScopes(grant.Scopes).
		SetWorkspaceID(grant.WorkspaceID).
		Save(ctx); err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": credentials.TokenValue(prefix, secret),
		"token_type":   "Bearer",
		"scope":        strings.Join(grant.Scopes, " "),
	})
}

// validRedirectURI accepts https URLs and http on a loopback host, with no
// fragment (RFC 6749 3.1.2, OAuth 2.1).
func validRedirectURI(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Fragment != "" || u.User != nil {
		return false
	}
	switch u.Scheme {
	case "https":
		return true
	case "http":
		host := u.Hostname()
		if host == "localhost" {
			return true
		}
		ip := net.ParseIP(host)
		return ip != nil && ip.IsLoopback()
	}
	return false
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

// writeOAuthError renders an RFC 6749 section 5.2 error response.
func writeOAuthError(w http.ResponseWriter, code int, kind, description string) {
	body := map[string]string{"error": kind}
	if description != "" {
		body["error_description"] = description
	}
	writeJSON(w, code, body)
}

func registrationError(w http.ResponseWriter, kind, description string) {
	writeOAuthError(w, http.StatusBadRequest, kind, description)
}
