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
	// ClaimPasskeyCheck is the Unix time, in seconds, of the last fresh Passkey check of the
	// session: a Passkey login or a step-up check (ADR 0052). Like ClaimProviderLogin it is a
	// stateless proof for a User with no password, and a reissue keeps the original time.
	ClaimPasskeyCheck = "passkey_check"
	// ClaimMethod is how the session was obtained (a Method). A token without a known method
	// is refused: the User signs in again (ADR 0052).
	ClaimMethod = "method"
)

// Method is how a session was obtained (ADR 0052). The Two-factor requirement reads it:
// a session is Strong when the person proved more than a password or a provider account.
type Method string

const (
	// MethodPassword is a login by email and password with no Second factor step.
	MethodPassword Method = "password"
	// MethodProvider is a login with a Sign-in provider (ADR 0048) with no Second factor step.
	MethodProvider Method = "provider"
	// MethodEmail is the session a verification link starts: it proves the mailbox only.
	MethodEmail Method = "email"
	// MethodSecondFactor is a login that passed a Second factor or Recovery code step.
	MethodSecondFactor Method = "second_factor"
	// MethodPasskey is a login with a Passkey, which verifies the person and replaces both steps.
	MethodPasskey Method = "passkey"
)

// Strong reports whether a session of this method meets a Workspace's Two-factor requirement
// without the User having a Second factor. It is the one reading of "is this session strong".
func (m Method) Strong() bool { return m == MethodSecondFactor || m == MethodPasskey }

func (m Method) known() bool {
	switch m {
	case MethodPassword, MethodProvider, MethodEmail, MethodSecondFactor, MethodPasskey:
		return true
	}
	return false
}

// Session is what a session token records about how it began: its Method, the time of the
// Sign-in provider login it began with (zero otherwise) and the time of its last Passkey
// check (zero when it has none). The two times are the proofs that stand in for a password.
type Session struct {
	Method          Method
	ProviderLoginAt time.Time
	PasskeyCheckAt  time.Time
}

// WithoutSecondFactor is the session after its Second factor is gone: a session that was
// strong only because it passed a Second factor step falls back to the weakest method it began
// with.
func (s Session) WithoutSecondFactor() Session {
	if s.Method != MethodSecondFactor {
		return s
	}
	return s.fallBack()
}

// WithoutPasskeys is the session after the User's last Passkey is gone: a Passkey session
// falls back to the weakest method it began with, and its Passkey check no longer stands in
// for a password.
func (s Session) WithoutPasskeys() Session {
	if s.Method != MethodPasskey {
		return s
	}
	s.PasskeyCheckAt = time.Time{}
	return s.fallBack()
}

// fallBack is the session as the password or the provider login it began with.
func (s Session) fallBack() Session {
	if s.ProviderLoginAt.IsZero() {
		s.Method = MethodPassword
	} else {
		s.Method = MethodProvider
	}
	return s
}

// SessionCookie is the name of the cookie that carries the session token; the
// /site contract's cookie security scheme reads it.
const SessionCookie = "JWT"

// sessionIssuer is the token issuer and audience of every session.
const sessionIssuer = "sphericon"

// Sessions issues and clears the site session (ADR 0020): a JWT signed with the
// instance secret, carried in an HttpOnly, SameSite=Lax cookie that lives as long as
// the token (SPHERICON_SESSION_TTL, no refresh). It is the one place a session is minted; the
// login operations hand its cookie to the client.
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

// Issue signs a session for u, stamped with its id, current session epoch and how it began,
// and returns the cookie that carries it. The provider login time rides the token, so a step
// that asks a passwordless User for proof can tell how recent the login was without a store.
func (s *Sessions) Issue(u *ent.User, sess Session) (*http.Cookie, error) {
	now := s.now()
	claims := gptoken.Claims{
		Issuer:    sessionIssuer,
		Audience:  jwt.ClaimStrings{sessionIssuer},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(s.ttl)),
		User:      &gptoken.User{Name: u.Email, ID: "user_" + strconv.FormatInt(u.ID, 10), Email: u.Email},
	}
	StampUser(&claims, u)
	claims.User.SetStrAttr(ClaimMethod, string(sess.Method))
	if !sess.ProviderLoginAt.IsZero() {
		claims.User.SetStrAttr(ClaimProviderLogin, strconv.FormatInt(sess.ProviderLoginAt.Unix(), 10))
	}
	if !sess.PasskeyCheckAt.IsZero() {
		claims.User.SetStrAttr(ClaimPasskeyCheck, strconv.FormatInt(sess.PasskeyCheckAt.Unix(), 10))
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

// SessionOf is how a raw session token began; ok is false for a token that does not verify,
// is expired or records no known method.
func (s *Sessions) SessionOf(raw string) (Session, bool) {
	if raw == "" {
		return Session{}, false
	}
	claims, err := s.verifier.parse(raw)
	if err != nil || claims.ExpiresAt == nil || !s.now().Before(claims.ExpiresAt.Time) {
		return Session{}, false
	}
	return sessionOf(claims)
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

// sessionOf reads how a session token began; ok is false when its method is missing or unknown.
func sessionOf(claims gptoken.Claims) (Session, bool) {
	if claims.User == nil {
		return Session{}, false
	}
	m := Method(claims.User.StrAttr(ClaimMethod))
	if !m.known() {
		return Session{}, false
	}
	sess := Session{Method: m}
	if sec, err := strconv.ParseInt(claims.User.StrAttr(ClaimProviderLogin), 10, 64); err == nil {
		sess.ProviderLoginAt = time.Unix(sec, 0)
	}
	if sec, err := strconv.ParseInt(claims.User.StrAttr(ClaimPasskeyCheck), 10, 64); err == nil {
		sess.PasskeyCheckAt = time.Unix(sec, 0)
	}
	return sess, true
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
