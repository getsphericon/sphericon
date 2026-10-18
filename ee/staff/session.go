package staff

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"time"

	gptoken "github.com/go-pkgz/auth/v2/token"
	"github.com/golang-jwt/jwt/v5"

	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ent"
	staffapi "github.com/getsphericon/sphericon/gen/staff"
	"github.com/getsphericon/sphericon/internal/paths"
)

// SessionCookie is the name of the cookie that carries a Staff member session. It differs
// from the site's `JWT`, so neither surface even reads the other's cookie; the secret
// and the issuer differ too, so a token of one never verifies as the other's.
const SessionCookie = "STAFF_JWT"

// sessionIssuer is the issuer and audience of every Staff member session.
const sessionIssuer = "sphericon-staff"

// cookiePath confines the cookie to the Staff surface.
const cookiePath = paths.StaffMember

const (
	claimStaffID = "oid"
	claimEpoch   = "epoch"
)

// ErrUnauthorized is the security handler's refusal of a missing or bad session.
var ErrUnauthorized = errors.New("staff: unauthorized")

// Sessions issues, clears and verifies Staff member sessions: a JWT signed with the
// Staff member secret in an HttpOnly cookie that lives SessionTTL, with no refresh and no
// "remember me". It is the one place a session is minted.
type Sessions struct {
	tokens *gptoken.Service
	ttl    time.Duration
	secure bool
	now    func() time.Time
	ent    *ent.Client
	lic    *licensekey.License
}

// NewSessions builds the issuer and verifier.
func NewSessions(client *ent.Client, lic *licensekey.License, cfg Config) *Sessions {
	now := cfg.Clock
	if now == nil {
		now = time.Now
	}
	return &Sessions{
		tokens: gptoken.NewService(gptoken.Opts{
			SecretReader: gptoken.SecretFunc(func(string) (string, error) { return cfg.Secret, nil }),
			Issuer:       sessionIssuer,
			DisableXSRF:  true,
		}),
		ttl: cfg.SessionTTL, secure: cfg.SecureCookies, now: now, ent: client, lic: lic,
	}
}

// Issue signs a session for op, stamped with its id and current session epoch, and
// returns the cookie that carries it.
func (s *Sessions) Issue(op *ent.StaffMember) (*http.Cookie, error) {
	now := s.now()
	claims := gptoken.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    sessionIssuer,
			Audience:  jwt.ClaimStrings{sessionIssuer},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.ttl)),
		},
		User: &gptoken.User{Name: op.Email, ID: "staff_" + strconv.FormatInt(op.ID, 10), Email: op.Email},
	}
	claims.User.SetStrAttr(claimStaffID, strconv.FormatInt(op.ID, 10))
	claims.User.SetStrAttr(claimEpoch, strconv.FormatInt(op.SessionEpoch, 10))
	tk, err := s.tokens.Token(claims)
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
	if !s.lic.Has(licensekey.FeatureOperator) {
		return nil, ErrUnauthorized
	}
	claims, err := s.tokens.Parse(raw)
	if err != nil || claims.Issuer != sessionIssuer || !slices.Contains(claims.Audience, sessionIssuer) {
		return nil, ErrUnauthorized
	}
	if claims.ExpiresAt == nil || !s.now().Before(claims.ExpiresAt.Time) || claims.User == nil {
		return nil, ErrUnauthorized
	}
	id, idErr := strconv.ParseInt(claims.User.StrAttr(claimStaffID), 10, 64)
	epoch, epochErr := strconv.ParseInt(claims.User.StrAttr(claimEpoch), 10, 64)
	if idErr != nil || epochErr != nil {
		return nil, ErrUnauthorized
	}
	op, err := s.ent.StaffMember.Get(ctx, id)
	if ent.IsNotFound(err) {
		return nil, ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	if op.SessionEpoch != epoch {
		return nil, ErrUnauthorized
	}
	return op, nil
}

type contextKey struct{}

// Current is the Staff member an authenticated request belongs to.
func Current(ctx context.Context) *ent.StaffMember {
	op, _ := ctx.Value(contextKey{}).(*ent.StaffMember)
	return op
}

// SecurityHandler implements staffapi.SecurityHandler: the Staff member cookie.
type SecurityHandler struct{ sessions *Sessions }

var _ staffapi.SecurityHandler = (*SecurityHandler)(nil)

// HandleApiKeyAuth authenticates the Staff member cookie and puts the Staff member in the
// context.
func (h *SecurityHandler) HandleApiKeyAuth(ctx context.Context, _ staffapi.OperationName, t staffapi.ApiKeyAuth) (context.Context, error) {
	op, err := h.sessions.Verify(ctx, t.APIKey)
	if err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, contextKey{}, op), nil
}
