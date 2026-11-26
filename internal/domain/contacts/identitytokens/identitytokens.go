// Package identitytokens is the proof an Identify carries (ADR 0171): the Workspace's Identity secret,
// which its server signs Identity tokens with, and the check of a token. An Identify through the
// public collect key proves nothing on its own; one with a token that verifies proves the alias keys
// the token names.
package identitytokens

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/identitysecret"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// MaxLifetime is how far ahead an Identity token may expire: a site mints one when it renders a page
// for a signed-in person, so a day is plenty, and a token copied from a page stops working soon.
const MaxLifetime = 24 * time.Hour

// leeway absorbs the clock drift between the customer's server and the Instance.
const leeway = time.Minute

// ErrTokenInvalid refuses an Identity token that does not verify: a bad signature or algorithm, no
// expiry or one past or too far ahead, no key named, or a Workspace with no Identity secret. It is
// never read as a claim instead, so a broken integration shows.
var ErrTokenInvalid = validation.Refuse("identity: the identity token does not verify", i18n.ErrorsIdentityTokenInvalid)

// Proof is the alias keys an Identity token proves: at least one of them.
type Proof struct {
	SubjectID, Email, Phone *string
}

// Module keeps the Identity secret and checks Identity tokens.
type Module struct {
	cipher *secrets.Cipher
	now    func() time.Time
}

// New builds the module; cipher seals the secret at rest and now is the clock a token's expiry is
// read against.
func New(cipher *secrets.Cipher, now func() time.Time) *Module {
	return &Module{cipher: cipher, now: now}
}

// Secret is the Workspace's Identity secret, "" while it has none.
func (m *Module) Secret(ctx context.Context, s *ent.Scoped) (string, error) {
	row, err := s.IdentitySecret().Query().Only(ctx)
	if ent.IsNotFound(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	plain, err := m.cipher.Decrypt(row.SecretEncrypted)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// Regenerate gives the Workspace a new Identity secret and returns it. The one before it stops
// verifying at once: regenerating is what a leaked secret calls for, and a token lives a day at most.
func (m *Module) Regenerate(ctx context.Context, s *ent.Scoped) (string, error) {
	secret := generate()
	sealed, err := m.cipher.Encrypt([]byte(secret))
	if err != nil {
		return "", err
	}
	_, err = s.IdentitySecret().Create().
		SetSecretEncrypted(sealed).
		OnConflictColumns(identitysecret.FieldWorkspaceID).
		DoNothing().
		ID(ctx)
	inserted, err := db.Inserted(err)
	if err != nil || inserted {
		return secret, err
	}
	if err := s.IdentitySecret().Update().SetSecretEncrypted(sealed).Exec(ctx); err != nil {
		return "", err
	}
	return secret, nil
}

// The unique constraint of the Identity secret (ADR 0106): one row per Workspace.
var _ = db.Declare(
	db.Fault("identitysecret_workspace_id", "Regenerate inserts the secret ON CONFLICT (workspace_id) DO NOTHING, then updates the one there"),
)

// claims is what an Identity token carries: `sub` is the subject id, beside the email and phone.
type claims struct {
	jwt.RegisteredClaims

	Email string `json:"email,omitempty"`
	Phone string `json:"phone,omitempty"`
}

// Verify checks an Identity token against the Workspace's secret and returns the keys it proves.
// ErrTokenInvalid when it does not verify.
func (m *Module) Verify(ctx context.Context, s *ent.Scoped, token string) (Proof, error) {
	secret, err := m.Secret(ctx, s)
	if err != nil {
		return Proof{}, err
	}
	if secret == "" {
		return Proof{}, ErrTokenInvalid
	}
	now := m.now()
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(leeway),
		jwt.WithTimeFunc(func() time.Time { return now }),
	)
	var c claims
	if _, err := parser.ParseWithClaims(token, &c, func(*jwt.Token) (any, error) { return []byte(secret), nil }); err != nil {
		return Proof{}, ErrTokenInvalid
	}
	if c.ExpiresAt.After(now.Add(MaxLifetime + leeway)) {
		return Proof{}, ErrTokenInvalid
	}
	p := Proof{SubjectID: lo.EmptyableToPtr(c.Subject), Email: lo.EmptyableToPtr(c.Email), Phone: lo.EmptyableToPtr(c.Phone)}
	if p.SubjectID == nil && p.Email == nil && p.Phone == nil {
		return Proof{}, ErrTokenInvalid
	}
	return p, nil
}

// generate is a new Identity secret: 32 random bytes under a prefix that secret scanners can match.
func generate() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return "idsec_" + base64.RawURLEncoding.EncodeToString(b)
}
