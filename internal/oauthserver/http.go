package oauthserver

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
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
// the MCP origin (the app origin unless SPHERICON_MCP_URL gives MCP its own host).
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

// New builds the server over the service; appURL is the public origin and the OAuth issuer,
// and mcpURL the origin of the MCP resource (the same as appURL on a single host).
func New(svc *Service, appURL, mcpURL string) *Server {
	return &Server{Service: svc, issuer: strings.TrimSuffix(appURL, "/"), resource: ResourceURL(mcpURL)}
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
		CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
		ClientIDMetadataDocumentSupported bool     `json:"client_id_metadata_document_supported"`
	}{
		AuthServerMeta: oauthex.AuthServerMeta{
			Issuer:                            s.issuer,
			AuthorizationEndpoint:             s.issuer + authorizePath,
			TokenEndpoint:                     s.issuer + tokenPath,
			RegistrationEndpoint:              s.issuer + registerPath,
			ScopesSupported:                   SupportedScopes(),
			ResponseTypesSupported:            []string{"code"},
			GrantTypesSupported:               []string{grantAuthorizationCode, grantRefreshToken},
			TokenEndpointAuthMethodsSupported: []string{"none"},
		},
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
	if len(meta.GrantTypes) > 0 && !slices.Contains(meta.GrantTypes, grantAuthorizationCode) {
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
			GrantTypes:              []string{grantAuthorizationCode, grantRefreshToken},
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
		return isLoopback(raw)
	}
	return false
}

// isLoopback reports whether a redirect URI points at this machine.
func isLoopback(raw string) bool {
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
