package signin

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Flow is the state of one sign-in between `start` and `callback`: what the callback needs
// to know the answer is for the flow this browser began. It travels in a short-lived signed
// cookie, never in the URL, and holds nothing the browser's owner may not see.
type Flow struct {
	Provider string
	// State is echoed by the provider and compared in constant time.
	State string
	// Verifier is the PKCE code verifier; the provider holds only its challenge.
	Verifier string
	// Nonce binds an OIDC id_token to this flow.
	Nonce string
	// ReturnTo is the validated relative in-app path to land on afterwards, "" for the home page.
	ReturnTo string
	// LinkUserID is the User a link flow adds the provider's subject to; 0 for a sign-in.
	LinkUserID int64
	// Invitation is the raw token of the Invitation link the flow began from, "" for none.
	Invitation string
}

// ErrFlow is a flow cookie that cannot be used: missing, forged, expired, or another
// provider's.
var ErrFlow = errors.New("signin: the flow cookie is missing or invalid")

// FlowCodec seals and opens the flow cookie's value: an HS256 JWT keyed by a key derived
// from the instance secret for this one purpose, so no other token verifies as a flow.
type FlowCodec struct {
	key []byte
	now func() time.Time
}

// NewFlowCodec builds the codec from the instance's JWT secret; now is the clock, nil
// meaning time.Now.
func NewFlowCodec(secret string, now func() time.Time) *FlowCodec {
	if now == nil {
		now = time.Now
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("signin_flow"))
	return &FlowCodec{key: mac.Sum(nil), now: now}
}

type flowClaims struct {
	jwt.RegisteredClaims
	Provider string `json:"p"`
	State    string `json:"s"`
	Verifier string `json:"v"`
	Nonce    string `json:"n"`
	ReturnTo string `json:"r,omitempty"`
	LinkUser int64  `json:"u,omitempty"`
	Invite   string `json:"i,omitempty"`
}

// Seal signs the flow, valid for ttl.
func (c *FlowCodec) Seal(f Flow, ttl time.Duration) (string, error) {
	claims := flowClaims{
		ExpiresAt: jwt.NewNumericDate(c.now().Add(ttl)),
		Provider:  f.Provider, State: f.State, Verifier: f.Verifier, Nonce: f.Nonce, ReturnTo: f.ReturnTo, LinkUser: f.LinkUserID, Invite: f.Invitation,
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, &claims).SignedString(c.key)
}

// Open verifies a cookie's value and returns its flow. It is ErrFlow for anything but a
// live flow sealed here for the provider.
func (c *FlowCodec) Open(raw, provider string) (Flow, error) {
	var claims flowClaims
	_, err := jwt.ParseWithClaims(raw, &claims, func(*jwt.Token) (any, error) { return c.key, nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired(), jwt.WithTimeFunc(c.now))
	if err != nil || claims.Provider != provider || claims.State == "" {
		return Flow{}, ErrFlow
	}
	return Flow{Provider: claims.Provider, State: claims.State, Verifier: claims.Verifier, Nonce: claims.Nonce, ReturnTo: claims.ReturnTo, LinkUserID: claims.LinkUser, Invitation: claims.Invite}, nil
}

// StateMatches compares the state the provider echoed with the flow's, in constant time.
func (f Flow) StateMatches(echoed string) bool {
	return echoed != "" && subtle.ConstantTimeCompare([]byte(f.State), []byte(echoed)) == 1
}
