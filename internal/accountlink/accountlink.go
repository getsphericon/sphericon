// Package accountlink mints the Account link token: a short-lived, asymmetrically
// signed proof of who a Workspace Owner is, handed to an external account page. The
// receiver verifies it with the public key set the app publishes, so no secret is
// shared. The app never calls the address and stores nothing about the receiver.
package accountlink

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/validation"
	"github.com/go-jose/go-jose/v4"
	"github.com/golang-jwt/jwt/v5"
)

// TTL is how long a token is valid.
const TTL = 2 * time.Minute

// JWKSPath is where the app publishes its public key set.
const JWKSPath = "/.well-known/jwks.json"

// ErrNotConfigured is the Account link of an installation that configured none: there is no
// page to open.
var ErrNotConfigured = validation.NotFound("accountlink: the account link is not configured", "errors.account_link_not_configured")

// Link signs Account link tokens for one configured address.
type Link struct {
	address string
	issuer  string
	key     ed25519.PrivateKey
	kid     string
	jwks    []byte
}

// New builds a Link for the external page at address, signing with the Ed25519 seed
// (32 bytes, standard base64). issuer names this installation in the token.
func New(address, seed, issuer string) (*Link, error) {
	u, err := url.Parse(address)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, errors.New("SPHERICON_ACCOUNT_LINK_URL must be an absolute http(s) URL")
	}
	raw, err := base64.StdEncoding.DecodeString(seed)
	if err != nil || len(raw) != ed25519.SeedSize {
		return nil, fmt.Errorf("SPHERICON_ACCOUNT_LINK_SIGNING_KEY must be a base64 %d-byte Ed25519 seed (e.g. `openssl rand -base64 32`)", ed25519.SeedSize)
	}
	key := ed25519.NewKeyFromSeed(raw)
	pub, ok := key.Public().(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("ed25519 key has a %T public key", key.Public())
	}
	sum := sha256.Sum256(pub)
	kid := hex.EncodeToString(sum[:8])
	set, err := json.Marshal(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
		Key: pub, KeyID: kid, Algorithm: string(jose.EdDSA), Use: "sig",
	}}})
	if err != nil {
		return nil, err
	}
	return &Link{address: address, issuer: issuer, key: key, kid: kid, jwks: set}, nil
}

// FromConfig builds the installation's Link. configured is false, with no Link and no
// error, when no address is set.
func FromConfig(cfg *config.Config) (link *Link, configured bool, err error) {
	if cfg.AccountLinkURL == "" {
		return nil, false, nil
	}
	link, err = New(cfg.AccountLinkURL, cfg.AccountLinkSigningKey, cfg.AppURL)
	return link, err == nil, err
}

// Address is the external page the link opens.
func (l *Link) Address() string { return l.address }

// JWKS is the public key set document the receiver verifies tokens with.
func (l *Link) JWKS() []byte { return l.jwks }

// Subject is who the token names.
type Subject struct {
	UserID      int64
	WorkspaceID int64
	Role        string
}

// Token signs a token for the subject at time now; it expires TTL later.
func (l *Link) Token(s Subject, now time.Time) (token string, expiresAt time.Time, err error) {
	expiresAt = now.Add(TTL)
	claims := jwt.MapClaims{
		"iss":          l.issuer,
		"sub":          strconv.FormatInt(s.UserID, 10),
		"aud":          l.address,
		"iat":          now.Unix(),
		"exp":          expiresAt.Unix(),
		"jti":          rand.Text(),
		"workspace_id": strconv.FormatInt(s.WorkspaceID, 10),
		"role":         s.Role,
	}
	t := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	t.Header["kid"] = l.kid
	token, err = t.SignedString(l.key)
	return token, expiresAt, err
}
