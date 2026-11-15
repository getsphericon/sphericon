package auth

import (
	"errors"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
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

// SessionClaims is the payload of a session token (ADR 0020, ADR 0147): the registered claims the
// parser validates (issuer, audience and an expiry it requires, against the Instance clock) and
// the session's own. A token whose own claims do not hold is refused by Validate, which the parser
// runs after the registered ones.
type SessionClaims struct {
	jwt.RegisteredClaims

	// UserID is the User's id, the lookup key of every request.
	UserID int64 `json:"uid,string"`
	// Epoch is the User's session_epoch at issuance; a bump ends every earlier session, and a
	// token without one is refused.
	Epoch *int64 `json:"epoch"`
	// Method is how the session was obtained. A token without a known method is refused: the User
	// signs in again (ADR 0052).
	Method Method `json:"method"`
	// ProviderLogin is the time of the Sign-in provider login the session began with (ADR 0048).
	// It is the stateless proof that stands in for the password of a User who has none; only a
	// provider login sets it, and a reissue keeps the original time.
	ProviderLogin *jwt.NumericDate `json:"provider_login,omitempty"`
	// PasskeyCheck is the time of the last fresh Passkey check of the session: a Passkey login or
	// a step-up check (ADR 0052). Like ProviderLogin it is a stateless proof for a User with no
	// password, and a reissue keeps the original time.
	PasskeyCheck *jwt.NumericDate `json:"passkey_check,omitempty"`
}

// NewSessionClaims are the claims of a session of u that begins at now and lives ttl: its id,
// current session epoch and how it began. It is the one shape of a session token: Issue signs it,
// and the test harness mints its tokens from it.
func NewSessionClaims(u *ent.User, sess Session, now time.Time, ttl time.Duration) SessionClaims {
	c := SessionClaims{
		Issuer:    sessionIssuer,
		Audience:  jwt.ClaimStrings{sessionIssuer},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		UserID:    u.ID,
		Epoch:     new(u.SessionEpoch),
		Method:    sess.Method,
	}
	if !sess.ProviderLoginAt.IsZero() {
		c.ProviderLogin = jwt.NewNumericDate(sess.ProviderLoginAt)
	}
	if !sess.PasskeyCheckAt.IsZero() {
		c.PasskeyCheck = jwt.NewNumericDate(sess.PasskeyCheckAt)
	}
	return c
}

var errSessionClaims = errors.New("auth: the session token names no User, epoch or known method")

// Validate refuses a token without a User id, an epoch or a known method (jwt.ClaimsValidator).
func (c SessionClaims) Validate() error {
	if c.UserID == 0 || c.Epoch == nil || !c.Method.known() {
		return errSessionClaims
	}
	return nil
}

// session is how the token's session began.
func (c SessionClaims) session() Session {
	sess := Session{Method: c.Method}
	if c.ProviderLogin != nil {
		sess.ProviderLoginAt = c.ProviderLogin.Time
	}
	if c.PasskeyCheck != nil {
		sess.PasskeyCheckAt = c.PasskeyCheck.Time
	}
	return sess
}

// Sessions issues and clears the site session (ADR 0020): a JWT signed with the
// instance secret, carried in an HttpOnly, SameSite=Lax cookie that lives as long as
// the token (SPHERICON_SESSION_TTL, no refresh). It is the one place a session is minted; the
// login operations hand its cookie to the client.
type Sessions struct {
	// key signs every session; parser verifies with it and then with each secondary secret.
	key    []byte
	parser sessionParser
	ttl    time.Duration
	secure bool
	now    func() time.Time
}

// NewSessions builds the issuer. secure sets the cookie's Secure attribute (the
// instance is served over HTTPS); now is the Instance clock of the token's expiry,
// which is required. A session is signed with jwtSecret and verified against it and then
// against each of verifyWith, the secondary key of a JWT secret rotation.
func NewSessions(jwtSecret string, ttl time.Duration, secure bool, now func() time.Time, verifyWith ...string) *Sessions {
	if now == nil {
		panic("auth: the sessions need the Instance clock")
	}
	return &Sessions{
		key:    []byte(jwtSecret),
		parser: newSessionParser(jwtSecret, verifyWith, now),
		ttl:    ttl,
		secure: secure,
		now:    now,
	}
}

// Issue signs a session for u, stamped with its id, current session epoch and how it began,
// and returns the cookie that carries it. The provider login time rides the token, so a step
// that asks a passwordless User for proof can tell how recent the login was without a store.
func (s *Sessions) Issue(u *ent.User, sess Session) (*http.Cookie, error) {
	claims := NewSessionClaims(u, sess, s.now(), s.ttl)
	tk, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.key)
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
	claims, err := s.parser.parse(raw)
	if err != nil {
		return 0, 0, false
	}
	return claims.UserID, *claims.Epoch, true
}

// SessionOf is how a raw session token began; ok is false for a token that does not verify,
// is expired or records no known method.
func (s *Sessions) SessionOf(raw string) (Session, bool) {
	claims, err := s.parser.parse(raw)
	if err != nil {
		return Session{}, false
	}
	return claims.session(), true
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

// sessionParser verifies a session token against the primary secret and then each secondary one,
// and validates its claims: HS256 only, this issuer and audience, an expiry that is required and
// not reached on the Instance clock, and the session's own claims (SessionClaims.Validate).
type sessionParser struct {
	keys   jwt.VerificationKeySet
	parser *jwt.Parser
}

func newSessionParser(primary string, secondary []string, now func() time.Time) sessionParser {
	secrets := append([]string{primary}, secondary...)
	return sessionParser{
		keys: jwt.VerificationKeySet{Keys: lo.Map(secrets, func(s string, _ int) jwt.VerificationKey { return []byte(s) })},
		parser: jwt.NewParser(
			jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
			jwt.WithIssuer(sessionIssuer),
			jwt.WithAudience(sessionIssuer),
			jwt.WithExpirationRequired(),
			jwt.WithTimeFunc(now),
		),
	}
}

func (p sessionParser) parse(raw string) (*SessionClaims, error) {
	var claims SessionClaims
	if _, err := p.parser.ParseWithClaims(raw, &claims, func(*jwt.Token) (any, error) { return p.keys, nil }); err != nil {
		return nil, err
	}
	return &claims, nil
}
