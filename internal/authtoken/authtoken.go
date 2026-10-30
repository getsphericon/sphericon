// Package authtoken mints and verifies the short-lived, purpose-scoped tokens
// behind the self-service account flows: password reset, email verification, and
// email change. Tokens are signed JWTs (HS256), reusing the app's JWT secret
// rather than hand-rolling a MAC — the same choice as internal/tracking.
//
// Two properties make the single secret safe to share across flows:
//
//   - A `prp` (purpose) claim is validated on parse, so a token minted for one
//     flow (say, signup verification) cannot be replayed against another (reset).
//   - The HMAC signing key is *derived per token* from (purpose, userID, binding),
//     where `binding` is a value that changes when the action completes — the
//     user's password hash for reset, their current email for a change. Once the
//     action lands, the binding no longer matches and the token stops verifying,
//     giving single-use semantics with no extra database column.
package authtoken

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// BindingError is the failure of the binding lookup a Parse was given: the lookup failed, which
// says nothing about the token. A caller tells a user that is gone (the token is stale) from a
// storage failure (the answer is a server error, not "invalid link") by what Err is.
type BindingError struct{ Err error }

func (e *BindingError) Error() string { return "authtoken: binding lookup: " + e.Err.Error() }

func (e *BindingError) Unwrap() error { return e.Err }

// Purpose scopes a token to one flow. It is both baked into the signing key and
// carried as a validated claim.
type Purpose string

const (
	PurposePasswordReset Purpose = "pwreset"
	PurposeEmailVerify   Purpose = "email_verify"
	PurposeEmailChange   Purpose = "email_change"
	// PurposeIdentityNotMe is the "this was not me" link of the notice mailed when a
	// Sign-in provider was linked silently (ADR 0048). It carries the Linked identity to
	// undo and has no binding: once the identity is gone the link has nothing left to do.
	PurposeIdentityNotMe Purpose = "identity_not_me"
	// PurposeUserDeletion is the one-time link mailed to a User with no password to confirm
	// their own User deletion (ADR 0050). Keyed to the User id and session epoch, never to
	// the address.
	PurposeUserDeletion Purpose = "user_deletion"
	// PurposeLoginChallenge is the challenge between the password step and the
	// Second factor step of a login (ADR 0020).
	PurposeLoginChallenge Purpose = "login_challenge"
	// PurposeStaffLoginChallenge is the same step for a Staff member (ADR 0026). Its
	// own purpose, and a signer on the Staff member secret, keep it from ever verifying
	// as a User's challenge: Staff member and User ids share no space.
	PurposeStaffLoginChallenge Purpose = "staff_login_challenge"
	// PurposePendingSignup is the signed reference of a Sign-in provider's sign-up held for the
	// Sign-up items (ADR 0062). Its id is the pending sign-up's, not a User's, and it is keyed to
	// the provider's subject: once the pending sign-up is gone the reference names nothing.
	PurposePendingSignup Purpose = "pending_signup"
)

// claims are the token's typed payload. The user id travels as a JSON string
// (`,string`) so it never passes through float64; extra carries purpose-specific
// string claims under one key.
type claims struct {
	jwt.RegisteredClaims

	UserID  int64             `json:"uid,string"`
	Purpose Purpose           `json:"prp"`
	Extra   map[string]string `json:"x,omitempty"`
}

// Signer mints and parses authtoken JWTs against a single secret.
type Signer struct {
	secret []byte
	now    func() time.Time
}

// New builds a Signer from the shared JWT secret, on the wall clock.
func New(secret string) *Signer {
	return &Signer{secret: []byte(secret), now: time.Now}
}

// WithClock returns a copy of the Signer that mints and checks expiry against now
// (tests move it past a token's lifetime instead of sleeping). nil keeps the clock.
func (s *Signer) WithClock(now func() time.Time) *Signer {
	if now == nil {
		return s
	}
	c := *s
	c.now = now
	return &c
}

// Mint signs a token for purpose + userID, keyed to binding (the value that
// changes when the action completes, giving single-use for free) and expiring
// after ttl. extra carries purpose-specific string claims (e.g. the requested new
// email for a change); nil is fine.
func (s *Signer) Mint(purpose Purpose, userID int64, binding string, ttl time.Duration, extra map[string]string) (string, error) {
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, &claims{
		ExpiresAt: jwt.NewNumericDate(s.now().Add(ttl)),
		UserID:    userID,
		Purpose:   purpose,
		Extra:     extra,
	})
	return tok.SignedString(s.deriveKey(purpose, userID, binding))
}

// Parse validates a token for the expected purpose and returns the user id plus
// any extra claims. bindingFor loads the current binding value for the decoded
// user id (e.g. their password hash) so a completed action invalidates the token;
// it is called with the claimed user id before the signature is verified.
func (s *Signer) Parse(token string, purpose Purpose, bindingFor func(userID int64) (string, error)) (userID int64, extra map[string]string, err error) {
	var c claims
	// golang-jwt hands keyFunc the parsed (not-yet-verified) claims, so we resolve
	// the per-user signing key here from the claimed uid + its current binding.
	_, err = jwt.ParseWithClaims(token, &c, func(tok *jwt.Token) (any, error) {
		cl, ok := tok.Claims.(*claims)
		if !ok {
			return nil, errors.New("authtoken: unexpected claims type")
		}
		if cl.Purpose != purpose {
			return nil, errors.New("authtoken: purpose mismatch")
		}
		if cl.UserID == 0 {
			return nil, errors.New("authtoken: missing uid claim")
		}
		binding, err := bindingFor(cl.UserID)
		if err != nil {
			return nil, &BindingError{Err: err}
		}
		return s.deriveKey(purpose, cl.UserID, binding), nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired(), jwt.WithTimeFunc(s.now))
	if err != nil {
		return 0, nil, err
	}
	if c.Extra == nil {
		c.Extra = map[string]string{}
	}
	return c.UserID, c.Extra, nil
}

// deriveKey binds the signing key to (purpose, userID, binding) so the token is
// scoped to one flow and one user, and stops verifying once the binding changes.
func (s *Signer) deriveKey(purpose Purpose, userID int64, binding string) []byte {
	mac := hmac.New(sha256.New, s.secret)
	// hash.Hash.Write never returns an error.
	_, _ = fmt.Fprintf(mac, "%s|%d|%s", purpose, userID, binding)
	return mac.Sum(nil)
}
