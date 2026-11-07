// Package oauth is the OAuth 2.1 authorization server for MCP connectors (ADR 0016, phase 2), a
// driver beside the API surfaces (ADR 0102): it serves the protocol's endpoints, the discovery
// documents (RFC 9728, RFC 8414), dynamic client registration (RFC 7591) through the OAuth clients
// module, the authorize step that hands the browser to the SPA consent screen, and the token
// endpoint, whose exchange of a code or a refresh token for a Connection is the use case
// internal/usecase/connectionissuance.
package oauth

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"

	"github.com/getsphericon/sphericon/internal/domain/identity/oauthclients"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
	"github.com/getsphericon/sphericon/internal/usecase/connectionissuance"
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
)

// ResourceURL is the canonical URI of the protected MCP resource, served from
// the MCP origin (the app origin unless SPHERICON_MCP_URL gives MCP its own host).
func ResourceURL(mcpOrigin string) string { return strings.TrimSuffix(mcpOrigin, "/") + mcpPath }

// ResourceMetadataURL is where /mcp points unauthenticated clients (RFC 9728),
// on the MCP origin.
func ResourceMetadataURL(mcpOrigin string) string {
	return strings.TrimSuffix(mcpOrigin, "/") + protectedResourcePath + mcpPath
}

const (
	grantAuthorizationCode = "authorization_code"
	grantRefreshToken      = "refresh_token"
)

// Server serves the OAuth protocol endpoints.
type Server struct {
	connections *connectionissuance.UseCase
	clients     *oauthclients.Module
	issuer      string
	resource    string
}

// New builds the server over the use case that issues a Connection and the OAuth clients module;
// appURL is the public origin and the OAuth issuer, and mcpURL the origin of the MCP resource (the
// same as appURL on a single host).
func New(connections *connectionissuance.UseCase, clients *oauthclients.Module, appURL, mcpURL string) *Server {
	return &Server{connections: connections, clients: clients, issuer: strings.TrimSuffix(appURL, "/"), resource: ResourceURL(mcpURL)}
}

// Mount registers discovery, registration, authorize and token endpoints.
func (s *Server) Mount(mux *http.ServeMux) {
	prm := auth.ProtectedResourceMetadataHandler(&oauthex.ProtectedResourceMetadata{
		Resource:               s.resource,
		AuthorizationServers:   []string{s.issuer},
		ScopesSupported:        oauthclients.SupportedScopes(),
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

		CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
		ClientIDMetadataDocumentSupported bool     `json:"client_id_metadata_document_supported"`
	}{
		Issuer:                            s.issuer,
		AuthorizationEndpoint:             s.issuer + authorizePath,
		TokenEndpoint:                     s.issuer + tokenPath,
		RegistrationEndpoint:              s.issuer + registerPath,
		ScopesSupported:                   oauthclients.SupportedScopes(),
		ResponseTypesSupported:            []string{"code"},
		GrantTypesSupported:               []string{grantAuthorizationCode, grantRefreshToken},
		TokenEndpointAuthMethodsSupported: []string{"none"},
		CodeChallengeMethodsSupported:     []string{"S256"},
		ClientIDMetadataDocumentSupported: true,
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
	if len(meta.RedirectURIs) == 0 || len(meta.RedirectURIs) > oauthclients.MaxRedirectURIs {
		registrationError(w, "invalid_redirect_uri", "provide between 1 and 10 redirect_uris")
		return
	}
	for _, u := range meta.RedirectURIs {
		if !oauthclients.ValidRedirectURI(u) {
			registrationError(w, "invalid_redirect_uri", "redirect_uris must be https URLs (or http on a loopback host) without a fragment")
			return
		}
	}
	if m := meta.TokenEndpointAuthMethod; m != "" && m != "none" {
		registrationError(w, "invalid_client_metadata", "only token_endpoint_auth_method none is supported (public clients with PKCE)")
		return
	}
	if len(meta.GrantTypes) > 0 && !slices.Contains(meta.GrantTypes, grantAuthorizationCode) {
		registrationError(w, "invalid_client_metadata", "grant_types must include authorization_code")
		return
	}

	row, err := s.clients.Register(r.Context(), meta.ClientName, meta.RedirectURIs)
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "")
		return
	}

	resp := oauthex.ClientRegistrationResponse{
		RedirectURIs:            row.RedirectUris,
		TokenEndpointAuthMethod: "none",
		GrantTypes:              []string{grantAuthorizationCode, grantRefreshToken},
		ResponseTypes:           []string{"code"},
		ClientName:              row.Name,
		ClientID:                row.ClientID,
		ClientIDIssuedAt:        row.CreatedAt,
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
	if _, err := s.clients.Client(r.Context(), q.Get("client_id"), redirectURI); err != nil {
		if errors.Is(err, oauthclients.ErrUnknownClient) || errors.Is(err, oauthclients.ErrInvalidRequest) {
			writeOAuthError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "")
		return
	}
	if kind, msg := checkAuthorizeParams(q, s.resource); msg != "" {
		target, err := oauthclients.RedirectWith(redirectURI, url.Values{"error": {kind}, "error_description": {msg}}, q.Get("state"))
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
	case q.Get("code_challenge_method") != "S256" || !oauthclients.ValidPKCE(q.Get("code_challenge")):
		return "invalid_request", "PKCE with code_challenge_method S256 is required"
	case q.Get("resource") != "" && q.Get("resource") != resource:
		return "invalid_target", "unknown resource"
	}
	return "", ""
}

// token serves the token endpoint: the authorization_code grant mints the Connection, the
// refresh_token grant rotates it.
func (s *Server) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "body must be form encoded")
		return
	}
	if res := r.PostForm.Get("resource"); res != "" && res != s.resource {
		writeOAuthError(w, http.StatusBadRequest, "invalid_target", "unknown resource")
		return
	}

	var (
		out *connectionissuance.Issued
		err error
	)
	switch r.PostForm.Get("grant_type") {
	case grantAuthorizationCode:
		out, err = s.connections.ExchangeCode(r.Context(), connectionissuance.ExchangeCodeInput{
			Code: r.PostForm.Get("code"), ClientID: r.PostForm.Get("client_id"),
			RedirectURI: r.PostForm.Get("redirect_uri"), Verifier: r.PostForm.Get("code_verifier"),
		})
	case grantRefreshToken:
		out, err = s.connections.Refresh(r.Context(), connectionissuance.RefreshInput{
			RefreshToken: r.PostForm.Get("refresh_token"), ClientID: r.PostForm.Get("client_id"), Scope: r.PostForm.Get("scope"),
		})
	default:
		writeOAuthError(w, http.StatusBadRequest, "unsupported_grant_type", "only authorization_code and refresh_token are supported")
		return
	}
	if code, ok := exchangeRefusal(err); ok {
		writeOAuthError(w, http.StatusBadRequest, code, validation.Message(err))
		return
	}
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  out.AccessToken,
		"token_type":    "Bearer",
		"expires_in":    int(connectionissuance.AccessTokenTTL.Seconds()),
		"refresh_token": out.RefreshToken,
		"scope":         strings.Join(out.Scopes, " "),
	})
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Warn("write json response", "error", err)
	}
}

// exchangeRefusal is the RFC 6749 section 5.2 error code of a refused exchange: a scope beyond the
// grant is invalid_scope, any other refusal invalid_grant. An error that is no refusal is not one.
func exchangeRefusal(err error) (string, bool) {
	switch {
	case errors.Is(err, connectionissuance.ErrScopeExceedsGrant):
		return "invalid_scope", true
	case errors.Is(err, connectionissuance.ErrInvalidGrant):
		return "invalid_grant", true
	}
	return "", false
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
