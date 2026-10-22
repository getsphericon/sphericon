// Package staff member is the platform Staff member (ADR 0008, ADR 0026): a staff identity
// outside the Workspace-scoped world, with its own store, its own two-step login
// (password, then a mandatory TOTP enrolled at first login, the pattern of ADR 0020),
// its own session cookie and JWT secret, and the /staff API surface.
//
// A Staff member belongs to no Workspace, so this package works on the raw *ent.Client
// (it is in AGENTS.md's closed list): the Staff member table, and the Workspace rows, the
// suspension state and the rates the console shows, which are tenant metadata rather
// than a Workspace's data. Two reads build a Workspace scope, each over the one
// Workspace the Staff member is looking at, because the module they ride takes one: the
// Audit log (Module.AuditEntries) and the reputation rates (Module.WorkspaceDeliverability).
// Every entry point checks the `staff` license feature, so an unlicensed instance
// has no Staff member concept at all.
//
// Governed by ee/LICENSE, not the AGPL.
package staff

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ee/abuse"
	"github.com/getsphericon/sphericon/ee/audit"
	"github.com/getsphericon/sphericon/ee/licensekey"
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
	// ErrNotLicensed: the instance has no `staff` license, so no Staff member exists.
	ErrNotLicensed = errors.New("staff: the staff feature is not licensed")
	// ErrDuplicate: a Staff member with this email exists.
	ErrDuplicate = errors.New("staff: email already in use")
	// ErrNotFound: no Staff member has this email.
	ErrNotFound = errors.New("staff: not found")
)

// Config is what the Staff member needs from the instance.
type Config struct {
	// Secret signs the Staff member session and challenge tokens (SPHERICON_STAFF_JWT_SECRET). It
	// is never the site's JWT secret.
	Secret string
	// SessionTTL is the lifetime of a Staff member session (SPHERICON_STAFF_SESSION_TTL).
	SessionTTL time.Duration
	// SecureCookies sets the cookie's Secure attribute (the instance is served over HTTPS).
	SecureCookies bool
	// Clock is the clock of TOTP codes, challenges and session expiry; nil is time.Now.
	Clock func() time.Time
	// Attempts counts failed logins per address (ADR 0025), under the Staff member's own
	// kind so a User of the same address never shares a counter. It is required with
	// the staff license (Validate): a missing counter would leave the login
	// unthrottled.
	Attempts *accounts.Attempts
	// Abuse are the abuse detector's limits (ADR 0026); the console shows rates under
	// the same volume floor the detector judges by. Zero is abuse.DefaultLimits.
	Abuse abuse.Limits
	// SupportEmail is the support contact an owner is given with a suspension notice
	// (SPHERICON_SUPPORT_EMAIL).
	SupportEmail string
	// HoldMaxExpiry is how far ahead a Staff hold's expiry may be set
	// (SPHERICON_STAFF_HOLD_MAX_EXPIRY); zero is DefaultHoldMaxExpiry.
	HoldMaxExpiry time.Duration
}

// Validate refuses a configuration a Staff surface cannot run on. It is checked
// only when the feature is licensed: an unlicensed instance needs none of it.
func (c Config) Validate(siteSecrets []string) error {
	if len(c.Secret) < minSecretLength {
		return fmt.Errorf("SPHERICON_STAFF_JWT_SECRET is required with the staff license and must be at least %d characters (e.g. `openssl rand -hex 32`)", minSecretLength)
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

// Module is the Staff member store and its login.
type Module struct {
	ent        *ent.Client
	lic        *licensekey.License
	audit      *audit.Log
	cipher     *secrets.Cipher
	challenges *authtoken.Signer
	login      *login.Module[*ent.StaffMember]
	now        func() time.Time
	watch      *abuse.Watch
	support    string
	holdMax    time.Duration
}

// NewModule builds the module over the raw client.
func NewModule(client *ent.Client, lic *licensekey.License, auditLog *audit.Log, cipher *secrets.Cipher, cfg Config) *Module {
	now := cfg.Clock
	if now == nil {
		now = time.Now
	}
	holdMax := cfg.HoldMaxExpiry
	if holdMax <= 0 {
		holdMax = DefaultHoldMaxExpiry
	}
	m := &Module{
		ent: client, lic: lic, audit: auditLog, cipher: cipher, now: now, watch: abuse.NewWatch(lic, cfg.Abuse), support: cfg.SupportEmail, holdMax: holdMax,
		challenges: authtoken.New(cfg.Secret).WithClock(now),
	}
	m.login = m.newLogin(cfg.Attempts.For(loginKind))
	return m
}

// Watch is the abuse detector's read of a Workspace's standing, with the limits this
// module shows rates under.
func (m *Module) Watch() *abuse.Watch { return m.watch }

func (m *Module) licensed() error {
	if !m.lic.Has(licensekey.FeatureStaff) {
		return ErrNotLicensed
	}
	return nil
}

// Create makes a Staff member with a random password, returned once (the platform owner
// hands it over; there is no signup). ErrDuplicate when the email is taken,
// ErrNotLicensed without the license.
func (m *Module) Create(ctx context.Context, email string) (password string, err error) {
	if err := m.licensed(); err != nil {
		return "", err
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
	hash, err := credentials.HashPassword(password)
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
	if err := m.licensed(); err != nil {
		return err
	}
	return m.ent.StaffMember.UpdateOneID(id).AddSessionEpoch(1).Exec(ctx)
}

// ResetTOTP is the only way a lost TOTP is reset (`sphericon staff reset-totp`, no
// web flow exists): it clears the secret and its confirmation, bumps the session epoch
// (which ends every session and kills any live challenge) and leaves the Staff member to
// re-enrol at next login. ErrNotFound for an unknown email.
func (m *Module) ResetTOTP(ctx context.Context, email string) error {
	if err := m.licensed(); err != nil {
		return err
	}
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
