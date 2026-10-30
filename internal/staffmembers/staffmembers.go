// Package staffmembers is the platform Staff member (ADR 0008, ADR 0026, ADR 0063): an identity
// outside the Workspace-scoped world that runs the Instance, with its own store, its own two-step
// login (password, then a mandatory TOTP enrolled at first login, the pattern of ADR 0020), and
// its own session cookie and JWT secret. It runs on every Instance, in both Modes and without a
// License key; the Enterprise sections of the console (ee/staff) build on it.
//
// A Staff member belongs to no Workspace, so this package works on the raw *ent.Client (it is in
// the closed list of docs/architecture.md): the Staff member table and nothing else.
package staffmembers

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent"
	entstaffmember "github.com/getsphericon/sphericon/ent/staffmember"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/authtoken"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/login"
	"github.com/getsphericon/sphericon/internal/secrets"
)

// Issuer names the instance in the authenticator app.
const Issuer = "Sphericon staff"

// challengeTTL is how long the password step's challenge stays valid.
const challengeTTL = 5 * time.Minute

// minSecretLength is the shortest SPHERICON_STAFF_JWT_SECRET accepted.
// contract-copy: a secret-strength floor for SPHERICON_STAFF_JWT_SECRET; no client sees it
const minSecretLength = 32

var (
	// ErrSignInOff: the Instance has no Staff member secret (SPHERICON_STAFF_JWT_SECRET), so no
	// Staff member can sign in and none is created.
	ErrSignInOff = errors.New("staff: sign-in is off without SPHERICON_STAFF_JWT_SECRET")
	// ErrDuplicate: a Staff member with this email exists.
	ErrDuplicate = errors.New("staff: email already in use")
	// ErrNotFound: no Staff member has this email.
	ErrNotFound = errors.New("staff: not found")
)

// Config is what the Staff member needs from the instance.
type Config struct {
	// Secret signs the Staff member session and challenge tokens (SPHERICON_STAFF_JWT_SECRET). It
	// is never the site's JWT secret. Empty turns the Staff sign-in off: the /staff surface is
	// not served and no Staff member is created.
	Secret string
	// SessionTTL is the lifetime of a Staff member session (SPHERICON_STAFF_SESSION_TTL).
	SessionTTL time.Duration
	// SecureCookies sets the cookie's Secure attribute (the instance is served over HTTPS).
	SecureCookies bool
	// Clock is the clock of TOTP codes, challenges and session expiry; nil is time.Now.
	Clock func() time.Time
	// Passwords hashes the password a Staff member is created with, and the login's decoy.
	Passwords *credentials.Hasher
	// Attempts counts failed logins per address (ADR 0025), under the Staff member's own
	// kind so a User of the same address never shares a counter. It is required with a
	// secret (Validate): a missing counter would leave the login unthrottled.
	Attempts *accounts.Attempts
}

// SignIn reports whether the Staff sign-in is on: the Instance has a Staff member secret.
func (c Config) SignIn() bool { return c.Secret != "" }

// Validate refuses a configuration the Staff sign-in cannot run on. With no secret the sign-in
// is off and there is nothing to check.
func (c Config) Validate(siteSecrets []string) error {
	if !c.SignIn() {
		return nil
	}
	if len(c.Secret) < minSecretLength {
		return fmt.Errorf("SPHERICON_STAFF_JWT_SECRET must be at least %d characters (e.g. `openssl rand -hex 32`)", minSecretLength)
	}
	for _, siteSecret := range siteSecrets {
		if subtle.ConstantTimeCompare([]byte(c.Secret), []byte(siteSecret)) == 1 {
			return errors.New("SPHERICON_STAFF_JWT_SECRET must differ from SPHERICON_JWT_SECRET and SPHERICON_JWT_SECRET_SECONDARY: a StaffMember session must never verify as a User's")
		}
	}
	if c.SessionTTL <= 0 {
		return errors.New("SPHERICON_STAFF_SESSION_TTL must be positive")
	}
	if c.Attempts == nil {
		return errors.New("the StaffMember login needs its failed-attempt counter: a missing one would leave it unthrottled")
	}
	return nil
}

// Module is the Staff member store, its login and its sessions.
type Module struct {
	ent        *ent.Client
	cipher     *secrets.Cipher
	challenges *authtoken.Signer
	login      *login.Module[*ent.StaffMember]
	sessions   *Sessions
	passwords  *credentials.Hasher
	now        func() time.Time
}

// New builds the module over the raw client, after checking cfg against the site's JWT
// secrets (Validate).
func New(client *ent.Client, cipher *secrets.Cipher, siteSecrets []string, cfg Config) (*Module, error) {
	if err := cfg.Validate(siteSecrets); err != nil {
		return nil, err
	}
	now := cfg.Clock
	if now == nil {
		now = time.Now
	}
	m := &Module{ent: client, cipher: cipher, passwords: cfg.Passwords, now: now}
	if cfg.SignIn() {
		m.challenges = authtoken.New(cfg.Secret).WithClock(now)
		m.login = m.newLogin(cfg.Attempts.For(loginKind))
		m.sessions = newSessions(client, cfg, now)
	}
	return m, nil
}

// Sessions issues and verifies Staff member sessions, nil while the sign-in is off.
func (m *Module) Sessions() *Sessions { return m.sessions }

// Create makes a Staff member with a random password, returned once (the platform owner
// hands it over; there is no signup). ErrDuplicate when the email is taken, ErrSignInOff
// without a Staff member secret.
func (m *Module) Create(ctx context.Context, email string) (password string, err error) {
	if m.sessions == nil {
		return "", ErrSignInOff
	}
	email = accounts.NormalizeEmail(email)
	if email == "" {
		return "", errors.New("staff: email is required")
	}
	exists, err := m.ent.StaffMember.Query().Where(entstaffmember.Email(email)).Exist(ctx)
	if err != nil {
		return "", err
	}
	if exists {
		return "", ErrDuplicate
	}
	password = rand.Text()
	hash, err := m.passwords.Hash(password)
	if err != nil {
		return "", err
	}
	err = m.ent.StaffMember.Create().SetEmail(email).SetPasswordHash(hash).Exec(ctx)
	if ent.IsConstraintError(err) {
		return "", ErrDuplicate
	}
	return password, err
}

// EndSessions ends every session of the Staff member by bumping its session epoch (and
// kills any live login challenge): a token copied elsewhere stops verifying too.
func (m *Module) EndSessions(ctx context.Context, id int64) error {
	return m.ent.StaffMember.UpdateOneID(id).AddSessionEpoch(1).Exec(ctx)
}

// ResetTOTP is the only way a lost TOTP is reset (`sphericon staff reset-totp`, no
// web flow exists): it clears the secret and its confirmation, bumps the session epoch
// (which ends every session and kills any live challenge) and leaves the Staff member to
// re-enrol at next login. ErrNotFound for an unknown email.
func (m *Module) ResetTOTP(ctx context.Context, email string) error {
	n, err := m.ent.StaffMember.Update().
		Where(entstaffmember.Email(accounts.NormalizeEmail(email))).
		SetTotpSecretEncrypted("").
		ClearTotpConfirmedAt().
		SetTotpLastStep(0).
		AddSessionEpoch(1).
		Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
