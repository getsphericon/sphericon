package auth

import (
	"net/http"
	"strconv"
	"time"

	"github.com/getsphericon/sphericon/ent"
	gptoken "github.com/go-pkgz/auth/v2/token"
	"github.com/golang-jwt/jwt/v5"
)

// The session token's claims beyond go-pkgz's own (ADR 0020). Both ride the token's
// user attributes as decimal strings: ClaimUserID is the User's id, the lookup key of
// every request; ClaimEpoch is the User's session_epoch at issuance.
const (
	ClaimUserID = "uid"
	ClaimEpoch  = "epoch"
	// ClaimProviderLogin is the Unix time, in seconds, of the Sign-in provider login the session
	// began with (ADR 0048). It is the stateless proof that stands in for the password of a User
	// who has none; only a provider login sets it, and a reissue keeps the original time.
	ClaimProviderLogin = "provider_login"
)

// SessionCookie is the name of the cookie that carries the session token; the
// /site contract's cookie security scheme reads it.
const SessionCookie = "JWT"

// sessionIssuer is the token issuer and audience of every session.
const sessionIssuer = "sphericon"

// Sessions issues and clears the site session (ADR 0020): a JWT signed with the
// instance secret, carried in an HttpOnly, SameSite=Lax cookie that lives as long as
// the token (SPHERICON_SESSION_TTL, no refresh). It is the one place a session is minted; the
// login operation, and later the Second factor step, hand its cookie to the client.
type Sessions struct {
	// verifier signs with its first service (the primary secret) and verifies with all.
	verifier verifier
	ttl      time.Duration
	secure   bool
	now      func() time.Time
}

// NewSessions builds the issuer. secure sets the cookie's Secure attribute (the
// instance is served over HTTPS); now is the clock of the token's expiry, nil
// meaning time.Now. A session is signed with jwtSecret and verified against it and then
// against each of verifyWith, the secondary key of a JWT secret rotation.
func NewSessions(jwtSecret string, ttl time.Duration, secure bool, now func() time.Time, verifyWith ...string) *Sessions {
	if now == nil {
		now = time.Now
	}
	return &Sessions{
		verifier: newVerifier(jwtSecret, verifyWith),
		ttl:      ttl,
		secure:   secure,
		now:      now,
	}
}

// Issue signs a session for u, stamped with its id and current session epoch, and
// returns the cookie that carries it.
func (s *Sessions) Issue(u *ent.User) (*http.Cookie, error) {
	return s.IssueAfterProviderLogin(u, time.Time{})
}

// IssueAfterProviderLogin is Issue for a session that began with a Sign-in provider login at
// providerLoginAt (the zero time for one that did not): the time rides the token, so a step
// that asks a passwordless User for proof can tell how recent the login was without a store.
func (s *Sessions) IssueAfterProviderLogin(u *ent.User, providerLoginAt time.Time) (*http.Cookie, error) {
	now := s.now()
	claims := gptoken.Claims{
		Issuer:    sessionIssuer,
		Audience:  jwt.ClaimStrings{sessionIssuer},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(s.ttl)),
		User:      &gptoken.User{Name: u.Email, ID: "user_" + strconv.FormatInt(u.ID, 10), Email: u.Email},
	}
	StampUser(&claims, u)
	if !providerLoginAt.IsZero() {
		claims.User.SetStrAttr(ClaimProviderLogin, strconv.FormatInt(providerLoginAt.Unix(), 10))
	}
	tk, err := s.verifier[0].Token(claims)
	if err != nil {
		return nil, err
	}
	return s.cookie(tk, int(s.ttl.Seconds())), nil
}

// Holder reads a raw session token the request carries without requiring one (a
// public operation that keeps the acting session when there is one): it checks the
// signature and expiry and returns the User id and epoch the token was issued
// under. The caller compares the epoch with the User's; ok is false for a token
// that does not verify.
func (s *Sessions) Holder(raw string) (id, epoch int64, ok bool) {
	if raw == "" {
		return 0, 0, false
	}
	claims, err := s.verifier.parse(raw)
	if err != nil || claims.ExpiresAt == nil || !s.now().Before(claims.ExpiresAt.Time) {
		return 0, 0, false
	}
	return sessionUser(claims)
}

// ProviderLoginOf is the time of the Sign-in provider login a raw session token began with;
// the zero time for a token that did not begin with one or does not verify.
func (s *Sessions) ProviderLoginOf(raw string) time.Time {
	if raw == "" {
		return time.Time{}
	}
	claims, err := s.verifier.parse(raw)
	if err != nil || claims.ExpiresAt == nil || !s.now().Before(claims.ExpiresAt.Time) {
		return time.Time{}
	}
	return providerLogin(claims)
}

// Cleared is the cookie that ends the session on the client: empty and expired.
func (s *Sessions) Cleared() *http.Cookie {
	c := s.cookie("", -1)
	c.Expires = time.Unix(0, 0)
	return c
}

func (s *Sessions) cookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name: SessionCookie, Value: value, Path: "/", MaxAge: maxAge,
		HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteLaxMode,
	}
}

// StampUser writes the User's id and session epoch into the claims' user: the
// claims every session carries (Issue; the test harness mints its tokens with it).
func StampUser(claims *gptoken.Claims, u *ent.User) {
	claims.User.SetStrAttr(ClaimUserID, strconv.FormatInt(u.ID, 10))
	claims.User.SetStrAttr(ClaimEpoch, strconv.FormatInt(u.SessionEpoch, 10))
}

// providerLogin reads the time of the Sign-in provider login a session token began with; the
// zero time when it did not.
func providerLogin(claims gptoken.Claims) time.Time {
	if claims.User == nil {
		return time.Time{}
	}
	sec, err := strconv.ParseInt(claims.User.StrAttr(ClaimProviderLogin), 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(sec, 0)
}

// sessionUser reads the User id and epoch a session token carries. ok is false
// when either is missing or malformed (a token issued before the epoch existed).
func sessionUser(claims gptoken.Claims) (id, epoch int64, ok bool) {
	if claims.User == nil {
		return 0, 0, false
	}
	id, err := strconv.ParseInt(claims.User.StrAttr(ClaimUserID), 10, 64)
	if err != nil {
		return 0, 0, false
	}
	epoch, err = strconv.ParseInt(claims.User.StrAttr(ClaimEpoch), 10, 64)
	if err != nil {
		return 0, 0, false
	}
	return id, epoch, true
}

// verifier parses a session token against the primary secret and then each secondary one.
type verifier []*gptoken.Service

func newTokenService(secret string) *gptoken.Service {
	return gptoken.NewService(gptoken.Opts{
		SecretReader: gptoken.SecretFunc(func(string) (string, error) { return secret, nil }),
		Issuer:       sessionIssuer,
		DisableXSRF:  true,
	})
}

func newVerifier(primary string, secondary []string) verifier {
	v := verifier{newTokenService(primary)}
	for _, secret := range secondary {
		v = append(v, newTokenService(secret))
	}
	return v
}

func (v verifier) parse(raw string) (gptoken.Claims, error) {
	var err error
	for _, svc := range v {
		var claims gptoken.Claims
		if claims, err = svc.Parse(raw); err == nil {
			return claims, nil
		}
	}
	return gptoken.Claims{}, err
}
