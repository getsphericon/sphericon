package staffmembers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/kernel/paths"
)

// SessionCookie is the name of the cookie that carries a Staff member session. It differs
// from the site's `JWT`, so neither surface even reads the other's cookie; the secret
// and the issuer differ too, so a token of one never verifies as the other's.
const SessionCookie = "STAFF_JWT"

// sessionIssuer is the issuer and audience of every Staff member session.
const sessionIssuer = "sphericon-staff"

// cookiePath confines the cookie to the Staff surface.
const cookiePath = paths.Staff

// ErrUnauthorized is the security handler's refusal of a missing or bad session.
var ErrUnauthorized = errors.New("staff: unauthorized")

// sessionClaims is the payload of a Staff member session (ADR 0147): the registered claims the
// parser validates and the Staff member's id and session epoch at issuance.
type sessionClaims struct {
	jwt.RegisteredClaims

	StaffID int64  `json:"oid,string"`
	Epoch   *int64 `json:"epoch"`
}

var errSessionClaims = errors.New("staff: the session token names no Staff member or epoch")

// Validate refuses a token without a Staff member id or an epoch (jwt.ClaimsValidator).
func (c sessionClaims) Validate() error {
	if c.StaffID == 0 || c.Epoch == nil {
		return errSessionClaims
	}
	return nil
}

// Sessions issues, clears and verifies Staff member sessions: a JWT signed with the
// Staff member secret in an HttpOnly cookie that lives SessionTTL, with no refresh and no
// "remember me". It is the one place a session is minted, and the one check of a session
// that the security handler of every /staff surface (core and Enterprise) calls.
type Sessions struct {
	key []byte
	// parser accepts HS256 only, this issuer and audience, and an expiry that is required and
	// not reached on the Instance clock.
	parser *jwt.Parser
	ttl    time.Duration
	secure bool
	now    func() time.Time
	ent    *ent.Client
}

// NewSessions builds the issuer and verifier of the sessions signed with cfg's secret, over the raw
// client, on cfg's Instance clock. The module builds them while its sign-in is on (Module.Sessions).
func NewSessions(client *ent.Client, cfg Config) *Sessions {
	now := cfg.Clock
	return &Sessions{
		key: []byte(cfg.Secret),
		parser: jwt.NewParser(
			jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
			jwt.WithIssuer(sessionIssuer),
			jwt.WithAudience(sessionIssuer),
			jwt.WithExpirationRequired(),
			jwt.WithTimeFunc(now),
		),
		ttl: cfg.SessionTTL, secure: cfg.SecureCookies, now: now, ent: client,
	}
}

// Issue signs a session for op, stamped with its id and current session epoch, and
// returns the cookie that carries it.
func (s *Sessions) Issue(op *ent.StaffMember) (*http.Cookie, error) {
	now := s.now()
	claims := sessionClaims{
		Issuer:    sessionIssuer,
		Audience:  jwt.ClaimStrings{sessionIssuer},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(s.ttl)),
		StaffID:   op.ID,
		Epoch:     new(op.SessionEpoch),
	}
	tk, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.key)
	if err != nil {
		return nil, err
	}
	return s.cookie(tk, int(s.ttl.Seconds())), nil
}

// Cleared is the cookie that ends the session on the client: empty and expired.
func (s *Sessions) Cleared() *http.Cookie {
	c := s.cookie("", -1)
	c.Expires = time.Unix(0, 0)
	return c
}

func (s *Sessions) cookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name: SessionCookie, Value: value, Path: cookiePath, MaxAge: maxAge,
		HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode,
	}
}

// Verify checks a raw session token (signature, issuer, expiry, Staff member id and epoch)
// and returns the Staff member it belongs to, or ErrUnauthorized.
func (s *Sessions) Verify(ctx context.Context, raw string) (*ent.StaffMember, error) {
	var claims sessionClaims
	if _, err := s.parser.ParseWithClaims(raw, &claims, func(*jwt.Token) (any, error) { return s.key, nil }); err != nil {
		return nil, ErrUnauthorized
	}
	op, err := s.ent.StaffMember.Get(ctx, claims.StaffID)
	if ent.IsNotFound(err) {
		return nil, ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	if op.SessionEpoch != *claims.Epoch {
		return nil, ErrUnauthorized
	}
	return op, nil
}
