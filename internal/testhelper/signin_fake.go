package testhelper

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/signin"
)

// SigninIdentity is who the fake provider says the person is: the subject, the email and
// whether it is verified, Google's `hd` claim, the name. Every flow of a test runs with
// one chosen here.
type SigninIdentity struct {
	Subject       string
	Email         string
	EmailVerified bool
	HostedDomain  string
	Name          string
	// Login is a GitHub account's handle, the name it falls back to.
	Login string
	// OtherEmails are the further addresses of a GitHub account, beside the primary one
	// (Email, EmailVerified).
	OtherEmails []SigninEmail
}

// SigninEmail is one more address of a GitHub account.
type SigninEmail struct {
	Email    string
	Verified bool
}

// FakeSignin is an httptest provider the Sign-in registry is pointed at: it serves the
// token and key endpoints of an OIDC provider, answers an authorization request with a code
// for the chosen identity, and checks what a real provider checks (client credentials, the
// redirect URI, PKCE S256). The routes live on one mux so a provider with more endpoints
// (GitHub's account and emails) is more handlers on it.
type FakeSignin struct {
	Server *httptest.Server
	// ClientID and ClientSecret are what the Instance is configured with.
	ClientID, ClientSecret string

	key *rsa.PrivateKey
	mux *http.ServeMux

	mu     sync.Mutex
	grants map[string]grant
	// accounts is who each issued access token belongs to, for GitHub's API.
	accounts map[string]SigninIdentity
}

type grant struct {
	identity    SigninIdentity
	nonce       string
	challenge   string
	redirectURI string
}

// NewFakeSignin starts the fake and stops it when the test ends.
func NewFakeSignin(t *testing.T) *FakeSignin {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	f := &FakeSignin{ClientID: "fake-client-id", ClientSecret: "fake-client-secret", key: key, mux: http.NewServeMux(), grants: map[string]grant{}, accounts: map[string]SigninIdentity{}}
	f.mux.HandleFunc("POST /token", f.token)
	f.mux.HandleFunc("GET /keys", f.keys)
	f.mux.HandleFunc("GET /user", f.githubUser)
	f.mux.HandleFunc("GET /user/emails", f.githubEmails)
	f.Server = httptest.NewServer(f.mux)
	t.Cleanup(f.Server.Close)
	return f
}

// Credentials is what the Instance is configured with for a provider.
func (f *FakeSignin) Credentials() config.SigninCredentials {
	return config.SigninCredentials{ClientID: f.ClientID, ClientSecret: f.ClientSecret}
}

// GitHubEndpoints points the registry's GitHub entry at the fake: plain OAuth2, with the
// account and its emails read from the API.
func (f *FakeSignin) GitHubEndpoints() signin.Endpoints {
	return signin.Endpoints{AuthURL: f.Server.URL + "/authorize", TokenURL: f.Server.URL + "/token", APIURL: f.Server.URL}
}

// Endpoints points the registry's Google entry at the fake.
func (f *FakeSignin) Endpoints() signin.Endpoints {
	return signin.Endpoints{AuthURL: f.Server.URL + "/authorize", TokenURL: f.Server.URL + "/token", Issuer: f.Server.URL, JWKSURL: f.Server.URL + "/keys"}
}

// Authorize is the person consenting at the provider: it takes the authorization URL `start`
// redirected to and returns the callback URL (path and query) the provider sends the browser
// back to, carrying a code for who and the request's own state.
func (f *FakeSignin) Authorize(t *testing.T, authURL string, who SigninIdentity) string {
	t.Helper()
	q := f.authRequest(t, authURL)
	code := rand.Text()
	f.mu.Lock()
	f.grants[code] = grant{identity: who, nonce: q.Get("nonce"), challenge: q.Get("code_challenge"), redirectURI: q.Get("redirect_uri")}
	f.mu.Unlock()
	return callbackURL(t, q.Get("redirect_uri"), url.Values{"code": {code}, "state": {q.Get("state")}})
}

// Deny is the person refusing consent: the callback URL carries the provider's error.
func (f *FakeSignin) Deny(t *testing.T, authURL string) string {
	t.Helper()
	q := f.authRequest(t, authURL)
	return callbackURL(t, q.Get("redirect_uri"), url.Values{"error": {"access_denied"}, "state": {q.Get("state")}})
}

func (f *FakeSignin) authRequest(t *testing.T, authURL string) url.Values {
	t.Helper()
	u, err := url.Parse(authURL)
	require.NoError(t, err)
	require.Equal(t, f.Server.URL+"/authorize", u.Scheme+"://"+u.Host+u.Path, "the browser was not sent to the fake provider")
	q := u.Query()
	require.Equal(t, f.ClientID, q.Get("client_id"))
	require.Equal(t, "code", q.Get("response_type"))
	require.Equal(t, "S256", q.Get("code_challenge_method"))
	return q
}

// callbackURL is the path and query of the redirect URI with the provider's answer added.
func callbackURL(t *testing.T, redirectURI string, answer url.Values) string {
	t.Helper()
	u, err := url.Parse(redirectURI)
	require.NoError(t, err)
	u.RawQuery = answer.Encode()
	return u.RequestURI()
}

func (f *FakeSignin) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || r.PostForm.Get("client_id") != f.ClientID || r.PostForm.Get("client_secret") != f.ClientSecret ||
		r.PostForm.Get("grant_type") != "authorization_code" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_client"})
		return
	}
	f.mu.Lock()
	g, ok := f.grants[r.PostForm.Get("code")]
	delete(f.grants, r.PostForm.Get("code"))
	f.mu.Unlock()
	sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
	if !ok || g.redirectURI != r.PostForm.Get("redirect_uri") || g.challenge != base64.RawURLEncoding.EncodeToString(sum[:]) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
		return
	}
	idToken, err := f.idToken(g)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server_error"})
		return
	}
	access := rand.Text()
	f.mu.Lock()
	f.accounts[access] = g.identity
	f.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"access_token": access, "token_type": "Bearer", "expires_in": 3600, "id_token": idToken})
}

// account is the identity a GitHub API call's bearer token was issued for.
func (f *FakeSignin) account(w http.ResponseWriter, r *http.Request) (SigninIdentity, bool) {
	f.mu.Lock()
	who, ok := f.accounts[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
	f.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "Bad credentials"})
	}
	return who, ok
}

func (f *FakeSignin) githubUser(w http.ResponseWriter, r *http.Request) {
	who, ok := f.account(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(who.Subject, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "the fake's GitHub subject is a number"})
		return
	}
	// GitHub's name is null for an account that set none.
	var name any
	if who.Name != "" {
		name = who.Name
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "login": who.Login, "name": name})
}

func (f *FakeSignin) githubEmails(w http.ResponseWriter, r *http.Request) {
	who, ok := f.account(w, r)
	if !ok {
		return
	}
	emails := []map[string]any{}
	if who.Email != "" {
		emails = append(emails, map[string]any{"email": who.Email, "primary": true, "verified": who.EmailVerified})
	}
	for _, o := range who.OtherEmails {
		emails = append(emails, map[string]any{"email": o.Email, "primary": false, "verified": o.Verified})
	}
	writeJSON(w, http.StatusOK, emails)
}

func (f *FakeSignin) idToken(g grant) (string, error) {
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: f.key, KeyID: "fake-key"}}, (&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		return "", err
	}
	now := time.Now()
	claims := map[string]any{
		"email": g.identity.Email, "email_verified": g.identity.EmailVerified, "name": g.identity.Name, "nonce": g.nonce,
	}
	if g.identity.HostedDomain != "" {
		claims["hd"] = g.identity.HostedDomain
	}
	return jwt.Signed(signer).Claims(jwt.Claims{
		Issuer: f.Server.URL, Subject: g.identity.Subject, Audience: jwt.Audience{f.ClientID},
		IssuedAt: jwt.NewNumericDate(now), Expiry: jwt.NewNumericDate(now.Add(time.Hour)),
	}).Claims(claims).Serialize()
}

func (f *FakeSignin) keys(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &f.key.PublicKey, KeyID: "fake-key", Algorithm: "RS256", Use: "sig"}}})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WithSignin enables the provider "google" on the test Instance, pointed at the fake.
func WithSignin(f *FakeSignin) Option {
	return func(s *setup) {
		enableSignin(s, "google", f.Credentials())
		s.signinOpts = append(s.signinOpts, signin.WithEndpoints("google", f.Endpoints()))
	}
}

// WithSigninGitHub enables the provider "github" on the test Instance, pointed at the fake.
func WithSigninGitHub(f *FakeSignin) Option {
	return func(s *setup) {
		enableSignin(s, "github", f.Credentials())
		s.signinOpts = append(s.signinOpts, signin.WithEndpoints("github", f.GitHubEndpoints()))
	}
}

func enableSignin(s *setup, name string, c config.SigninCredentials) {
	// The config a test starts from is shared, so the map is replaced, never written to.
	s.cfg.Signin = maps.Clone(s.cfg.Signin)
	if s.cfg.Signin == nil {
		s.cfg.Signin = map[string]config.SigninCredentials{}
	}
	s.cfg.Signin[name] = c
}
