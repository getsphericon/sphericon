// Package signin is the Sign-in provider registry and the OAuth/OIDC round trip of a
// provider sign-in (ADR 0048). It knows nothing of Users: it starts a flow, turns the
// provider's answer into a Profile and says whether the provider's proof of the email
// address is accepted. The sign-in decision itself lives in internal/accounts.
//
// A provider is one entry of the catalog (name, endpoints, scopes, how its Profile is read
// and how far its email is trusted), so adding a provider is adding an entry. An entry is
// live only when the Instance configured its client id and secret.
package signin

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/getsphericon/sphericon/config"
)

// Reason is why a provider sign-in did not start a session. Its values are the
// SiteSigninError enum of the /site contract: the callback redirects to the login page
// with one as the `error` query parameter.
type Reason string

const (
	ReasonDenied              Reason = "denied"
	ReasonProviderError       Reason = "provider_error"
	ReasonInvalidState        Reason = "invalid_state"
	ReasonProviderUnavailable Reason = "provider_unavailable"
	ReasonEmailUnverified     Reason = "email_unverified"
	ReasonEmailNotTrusted     Reason = "email_not_trusted"
	ReasonSignupClosed        Reason = "signup_closed"
	ReasonIdentityInUse       Reason = "identity_in_use"
	// ReasonInvitationEmailMismatch: accepting an Invitation needs the provider's trusted
	// email to be the invited address. ReasonInvitationInvalid: the Invitation is not pending.
	ReasonInvitationEmailMismatch Reason = "invitation_email_mismatch"
	ReasonInvitationInvalid       Reason = "invitation_invalid"
)

// Reasons lists every Reason, for the test that keeps them equal to the contract's enum.
func Reasons() []Reason {
	return []Reason{
		ReasonDenied, ReasonProviderError, ReasonInvalidState, ReasonProviderUnavailable,
		ReasonEmailUnverified, ReasonEmailNotTrusted, ReasonSignupClosed, ReasonIdentityInUse,
		ReasonInvitationEmailMismatch, ReasonInvitationInvalid,
	}
}

// Profile is what a provider says about the person who signed in. Subject is the
// provider's immutable identifier of the account: the only key a Linked identity is found by.
type Profile struct {
	Subject string
	Name    string
	Email   string
	// EmailVerified is the provider's own claim that the person controls Email.
	EmailVerified bool
	// HostedDomain is Google's `hd` claim: the Workspace domain that owns the account.
	HostedDomain string
}

// Endpoints are where a provider is reached. They are data of the catalog entry; a test
// points them at its fake provider.
type Endpoints struct {
	AuthURL  string
	TokenURL string
	// Issuer and JWKSURL verify an OIDC id_token; empty for a provider that is plain OAuth2.
	Issuer  string
	JWKSURL string
	// APIURL is the base of a plain OAuth2 provider's own API, where the account is read.
	APIURL string
}

// Exchange is what an entry reads a Profile from: the tokens the provider issued for the
// code, and an HTTP client for the provider's own APIs (a GitHub-style provider asks them
// for the account and its emails).
type Exchange struct {
	Token *oauth2.Token
	// Nonce is the flow's nonce, which an OIDC id_token must echo.
	Nonce string
	// Verifier checks an id_token; nil for a provider that is plain OAuth2.
	Verifier *oidc.IDTokenVerifier
	// HTTP is a client that sends the access token to the provider's APIs.
	HTTP *http.Client
	// APIURL is the base of the provider's API (Endpoints.APIURL).
	APIURL string
}

// Entry is one Sign-in provider of the catalog.
type Entry struct {
	// Name is the registry key: the path segment of the operations, the provider column of
	// a Linked identity and the word of the env variables.
	Name string
	// DisplayName is what the button says.
	DisplayName string
	Scopes      []string
	Endpoints   Endpoints
	// Profile reads the person from the provider's answer.
	Profile func(ctx context.Context, x Exchange) (Profile, error)
	// Trust says whether the provider's proof of the email is accepted: "" for yes, else
	// the Reason it is refused.
	Trust func(Profile) Reason
}

// catalog is every provider this build knows. Adding a provider is one entry here (and the
// two variables its credentials are read from, in config).
func catalog() []Entry {
	return []Entry{google(), github()}
}

// Info is what the public listing says about an enabled provider.
type Info struct {
	Name        string
	DisplayName string
}

// Providers is the set of providers an Instance enabled.
type Providers struct {
	live  []*provider
	http  *http.Client
	flows *FlowCodec
}

type provider struct {
	entry    Entry
	oauth    *oauth2.Config
	verifier *oidc.IDTokenVerifier
}

// Option tunes New.
type Option func(*options)

type options struct {
	endpoints map[string]Endpoints
	http      *http.Client
	flows     *FlowCodec
}

// WithFlowCodec sets what seals the flow cookie. Without it the codec is keyed by a secret
// drawn at start, which only a single process can open: an Instance passes its JWT secret
// (FromConfig).
func WithFlowCodec(c *FlowCodec) Option { return func(o *options) { o.flows = c } }

// WithEndpoints replaces where one provider is reached. A test uses it to point a provider
// at its fake; nothing else does.
func WithEndpoints(name string, e Endpoints) Option {
	return func(o *options) { o.endpoints[name] = e }
}

// WithHTTPClient replaces the client that talks to the providers.
func WithHTTPClient(c *http.Client) Option { return func(o *options) { o.http = c } }

// providerTimeout bounds every call to a provider.
const providerTimeout = 15 * time.Second

// FlowTTL is how long a started flow stays valid: the time a person takes at the provider.
const FlowTTL = 10 * time.Minute

// New enables the catalog entries that have credentials. appURL is the public address of
// the Instance, which the redirect URI each provider is told to use derives from.
func New(creds map[string]config.SigninCredentials, appURL string, opts ...Option) *Providers {
	o := options{endpoints: map[string]Endpoints{}, http: &http.Client{Timeout: providerTimeout}}
	for _, opt := range opts {
		opt(&o)
	}
	if o.flows == nil {
		o.flows = NewFlowCodec(rand.Text()+rand.Text(), nil)
	}
	p := &Providers{http: o.http, flows: o.flows}
	entries := catalog()
	for i := range entries {
		entry := &entries[i]
		c, ok := creds[entry.Name]
		if !ok {
			continue
		}
		if e, ok := o.endpoints[entry.Name]; ok {
			entry.Endpoints = e
		}
		live := &provider{
			entry: *entry,
			oauth: &oauth2.Config{
				ClientID:     c.ClientID,
				ClientSecret: c.ClientSecret,
				Endpoint:     oauth2.Endpoint{AuthURL: entry.Endpoints.AuthURL, TokenURL: entry.Endpoints.TokenURL, AuthStyle: oauth2.AuthStyleInParams},
				RedirectURL:  RedirectURL(appURL, entry.Name),
				Scopes:       entry.Scopes,
			},
		}
		if entry.Endpoints.Issuer != "" {
			keys := &lazyKeys{url: entry.Endpoints.JWKSURL, http: o.http}
			live.verifier = oidc.NewVerifier(entry.Endpoints.Issuer, keys, &oidc.Config{ClientID: c.ClientID})
		}
		p.live = append(p.live, live)
	}
	return p
}

// lazyKeys is the provider's signing keys, fetched from its JWKS URL on first use and cached
// by go-oidc's remote key set. The set outlives the request that first needs it, so it is
// built from that request's context without its cancellation.
type lazyKeys struct {
	url  string
	http *http.Client
	once sync.Once
	keys *oidc.RemoteKeySet
}

func (l *lazyKeys) VerifySignature(ctx context.Context, jwt string) ([]byte, error) {
	l.once.Do(func() {
		l.keys = oidc.NewRemoteKeySet(oidc.ClientContext(context.WithoutCancel(ctx), l.http), l.url)
	})
	return l.keys.VerifySignature(ctx, jwt)
}

// FromConfig enables the providers the Instance configured, on its public address, sealing
// flows with its JWT secret on the given clock (nil meaning time.Now).
func FromConfig(cfg *config.Config, now func() time.Time, opts ...Option) *Providers {
	opts = append([]Option{WithFlowCodec(NewFlowCodec(cfg.JWTSecret, now))}, opts...)
	return New(cfg.Signin, cfg.AppURL, opts...)
}

// SealFlow wraps a started flow for the browser's cookie, valid for FlowTTL.
func (p *Providers) SealFlow(f Flow) (string, error) { return p.flows.Seal(f, FlowTTL) }

// OpenFlow reads the flow cookie of a callback for the provider; ErrFlow when it cannot be used.
func (p *Providers) OpenFlow(raw, provider string) (Flow, error) { return p.flows.Open(raw, provider) }

// RedirectURL is the redirect URI a provider is told to send the browser back to: the
// callback operation under the public app URL. It is what an operator registers with the
// provider.
func RedirectURL(appURL, name string) string {
	return strings.TrimRight(appURL, "/") + "/site/auth/signin/" + name + "/callback"
}

// Enabled lists the enabled providers in catalog order.
func (p *Providers) Enabled() []Info {
	out := make([]Info, len(p.live))
	for i, l := range p.live {
		out[i] = Info{Name: l.entry.Name, DisplayName: l.entry.DisplayName}
	}
	return out
}

func (p *Providers) find(name string) *provider {
	i := slices.IndexFunc(p.live, func(l *provider) bool { return l.entry.Name == name })
	if i < 0 {
		return nil
	}
	return p.live[i]
}

// Has reports whether the provider is enabled.
func (p *Providers) Has(name string) bool { return p.find(name) != nil }

// Start draws a flow for the provider and returns the URL to send the browser to. The flow
// (state, PKCE verifier, nonce) is the caller's to keep until the callback; returnTo is
// carried in it untouched.
func (p *Providers) Start(name, returnTo string) (authURL string, flow Flow, ok bool) {
	l := p.find(name)
	if l == nil {
		return "", Flow{}, false
	}
	flow = Flow{Provider: name, State: rand.Text(), Verifier: oauth2.GenerateVerifier(), Nonce: rand.Text(), ReturnTo: returnTo}
	opts := []oauth2.AuthCodeOption{oauth2.S256ChallengeOption(flow.Verifier)}
	if l.verifier != nil {
		opts = append(opts, oidc.Nonce(flow.Nonce))
	}
	return l.oauth.AuthCodeURL(flow.State, opts...), flow, true
}

// ErrProvider is a failure of the provider's answer: a code it refused, a token that did not
// verify, a profile that could not be read. The caller shows it as ReasonProviderError.
var ErrProvider = errors.New("signin: the provider's answer could not be used")

// Complete trades the callback's code for the person's Profile.
func (p *Providers) Complete(ctx context.Context, flow Flow, code string) (Profile, error) {
	l := p.find(flow.Provider)
	if l == nil {
		return Profile{}, fmt.Errorf("%w: provider %q is not enabled", ErrProvider, flow.Provider)
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, p.http)
	ctx = oidc.ClientContext(ctx, p.http)
	token, err := l.oauth.Exchange(ctx, code, oauth2.VerifierOption(flow.Verifier))
	if err != nil {
		return Profile{}, fmt.Errorf("%w: %w", ErrProvider, err)
	}
	profile, err := l.entry.Profile(ctx, Exchange{Token: token, Nonce: flow.Nonce, Verifier: l.verifier, HTTP: l.oauth.Client(ctx, token), APIURL: l.entry.Endpoints.APIURL})
	if err != nil {
		return Profile{}, fmt.Errorf("%w: %w", ErrProvider, err)
	}
	return profile, nil
}

// Trust says whether the provider's proof of the Profile's email is accepted: "" for yes,
// else the Reason it is refused. An unknown provider trusts nothing.
func (p *Providers) Trust(name string, profile Profile) Reason {
	l := p.find(name)
	if l == nil {
		return ReasonProviderUnavailable
	}
	return l.entry.Trust(profile)
}

// DisplayName is what a person calls the provider: the catalog's name for it, or the registry
// name for one this build no longer knows.
func DisplayName(name string) string {
	entries := catalog()
	for i := range entries {
		if entries[i].Name == name {
			return entries[i].DisplayName
		}
	}
	return name
}
