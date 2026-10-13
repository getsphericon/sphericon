// Package operator is the platform Operator (ADR 0008, ADR 0026): a staff identity
// outside the Workspace-scoped world, with its own store, its own two-step login
// (password, then a mandatory TOTP enrolled at first login, the pattern of ADR 0020),
// its own session cookie and JWT secret, and the /operator API surface.
//
// An Operator belongs to no Workspace, so this package works on the raw *ent.Client
// (it is in AGENTS.md's closed list): the Operator table, and the Workspace rows, the
// suspension state and the rates the console shows, which are tenant metadata rather
// than a Workspace's data. Two reads build a Workspace scope, each over the one
// Workspace the Operator is looking at, because the module they ride takes one: the
// Audit log (Module.AuditEntries) and the reputation rates (Module.WorkspaceDeliverability).
// Every entry point checks the `operator` license feature, so an unlicensed instance
// has no Operator concept at all.
//
// Governed by ee/LICENSE, not the AGPL.
package operator

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ee/abuse"
	"github.com/getsphericon/sphericon/ee/audit"
	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ent"
	entoperator "github.com/getsphericon/sphericon/ent/operator"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/authtoken"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/otpcode"
	"github.com/getsphericon/sphericon/internal/secrets"
)

// Issuer names the instance in the authenticator app.
const Issuer = "sphericon staff"

// challengeTTL is how long the password step's challenge stays valid.
const challengeTTL = 5 * time.Minute

// minSecretLength is the shortest OPERATOR_JWT_SECRET accepted.
const minSecretLength = 32

var (
	// ErrNotLicensed: the instance has no `operator` license, so no Operator exists.
	ErrNotLicensed = errors.New("operator: the operator feature is not licensed")
	// ErrDuplicate: an Operator with this email exists.
	ErrDuplicate = errors.New("operator: email already in use")
	// ErrInvalidCredentials: the email and password match no Operator.
	ErrInvalidCredentials = errors.New("operator: invalid credentials")
	// ErrInvalidChallenge: the challenge is forged, expired, already used or its
	// Operator is gone.
	ErrInvalidChallenge = errors.New("operator: invalid challenge")
	// ErrInvalidCode: the code is not a current, unused TOTP code of the Operator.
	ErrInvalidCode = errors.New("operator: invalid code")
	// ErrNotFound: no Operator has this email.
	ErrNotFound = errors.New("operator: not found")
)

// ThrottledError is the login throttle's refusal (ADR 0025, ADR 0026): the address has
// failed too often and must wait Wait before the next attempt, even with the right
// password or code. The handler renders it as the standard 429.
type ThrottledError struct {
	// Wait is how long the address must still wait.
	Wait time.Duration
	// Limit is the failure threshold the throttle applies.
	Limit int
	// Now is the clock of the throttle, so the reset time is on its time base.
	Now time.Time
}

func (e *ThrottledError) Error() string {
	return fmt.Sprintf("operator: login throttled for %s", e.Wait)
}

// Config is what the Operator needs from the instance.
type Config struct {
	// Secret signs the Operator session and challenge tokens (OPERATOR_JWT_SECRET). It
	// is never the site's JWT secret.
	Secret string
	// SessionTTL is the lifetime of an Operator session (OPERATOR_SESSION_TTL).
	SessionTTL time.Duration
	// SecureCookies sets the cookie's Secure attribute (the instance is served over HTTPS).
	SecureCookies bool
	// Clock is the clock of TOTP codes, challenges and session expiry; nil is time.Now.
	Clock func() time.Time
	// Attempts counts failed logins per address (ADR 0025), under the Operator's own
	// kind so a User of the same address never shares a counter. It is required with
	// the operator license (Validate): a missing counter would leave the login
	// unthrottled.
	Attempts *accounts.Attempts
	// Abuse are the abuse detector's limits (ADR 0026); the console shows rates under
	// the same volume floor the detector judges by. Zero is abuse.DefaultLimits.
	Abuse abuse.Limits
	// SupportEmail is the support contact an owner is given with a suspension notice
	// (SUPPORT_EMAIL).
	SupportEmail string
	// HoldMaxExpiry is how far ahead an Operator hold's expiry may be set
	// (OPERATOR_HOLD_MAX_EXPIRY); zero is DefaultHoldMaxExpiry.
	HoldMaxExpiry time.Duration
}

// Validate refuses a configuration an Operator surface cannot run on. It is checked
// only when the feature is licensed: an unlicensed instance needs none of it.
func (c Config) Validate(siteSecret string) error {
	if len(c.Secret) < minSecretLength {
		return fmt.Errorf("OPERATOR_JWT_SECRET is required with the operator license and must be at least %d characters (e.g. `openssl rand -hex 32`)", minSecretLength)
	}
	if subtle.ConstantTimeCompare([]byte(c.Secret), []byte(siteSecret)) == 1 {
		return errors.New("OPERATOR_JWT_SECRET must differ from JWT_SECRET: an Operator session must never verify as a User's")
	}
	if c.SessionTTL <= 0 {
		return errors.New("OPERATOR_SESSION_TTL must be positive")
	}
	if c.Attempts == nil {
		return errors.New("the Operator login needs its failed-attempt counter: a missing one would leave it unthrottled")
	}
	return nil
}

// Module is the Operator store and its login.
type Module struct {
	ent        *ent.Client
	lic        *licensekey.License
	audit      *audit.Log
	cipher     *secrets.Cipher
	challenges *authtoken.Signer
	attempts   *accounts.Attempts
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
	return &Module{
		ent: client, lic: lic, audit: auditLog, cipher: cipher, attempts: cfg.Attempts, now: now, watch: abuse.NewWatch(lic, cfg.Abuse), support: cfg.SupportEmail, holdMax: holdMax,
		challenges: authtoken.New(cfg.Secret).WithClock(now),
	}
}

// Watch is the abuse detector's read of a Workspace's standing, with the limits this
// module shows rates under.
func (m *Module) Watch() *abuse.Watch { return m.watch }

func (m *Module) licensed() error {
	if !m.lic.Has(licensekey.FeatureOperator) {
		return ErrNotLicensed
	}
	return nil
}

func normalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

// Create makes an Operator with a random password, returned once (the platform owner
// hands it over; there is no signup). ErrDuplicate when the email is taken,
// ErrNotLicensed without the license.
func (m *Module) Create(ctx context.Context, email string) (password string, err error) {
	if err := m.licensed(); err != nil {
		return "", err
	}
	email = normalizeEmail(email)
	if email == "" {
		return "", errors.New("operator: email is required")
	}
	exists, err := m.ent.Operator.Query().Where(entoperator.Email(email)).Exist(ctx)
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
	err = m.ent.Operator.Create().SetEmail(email).SetPasswordHash(hash).Exec(ctx)
	if ent.IsConstraintError(err) {
		return "", ErrDuplicate
	}
	return password, err
}

// loginKind is the Kind under which an Operator's failed logins are counted.
const loginKind = accounts.KindOperatorLogin

// throttle refuses with a *ThrottledError while the address's delay runs.
func (m *Module) throttle(ctx context.Context, email string) error {
	wait, err := m.attempts.Delay(ctx, loginKind, email)
	if err != nil {
		return err
	}
	if wait > 0 {
		return &ThrottledError{Wait: wait, Limit: m.attempts.Limit(loginKind), Now: m.attempts.Now()}
	}
	return nil
}

// failed counts a failure of the address's login and returns cause with any error of
// the counting itself.
func (m *Module) failed(ctx context.Context, email string, cause error) error {
	return errors.Join(cause, m.attempts.RecordFailure(ctx, loginKind, email))
}

// CheckPassword is the first login step: it returns the Operator the credentials
// belong to, or ErrInvalidCredentials. While the address's failure delay runs it
// answers a *ThrottledError even for the right password, and every failure (an unknown
// email too) feeds the counter. A correct password does not reset it: only a started
// session does, else knowing the password would buy a fresh round of code guesses. A password is always hashed, against a
// throwaway hash when no Operator matches, so the answer time does not tell an unknown
// email apart.
func (m *Module) CheckPassword(ctx context.Context, email, password string) (*ent.Operator, error) {
	if err := m.licensed(); err != nil {
		return nil, err
	}
	email = normalizeEmail(email)
	if err := m.throttle(ctx, email); err != nil {
		return nil, err
	}
	op, err := m.ent.Operator.Query().Where(entoperator.Email(email)).Only(ctx)
	if ent.IsNotFound(err) {
		credentials.VerifyPassword(decoyHash(), password)
		return nil, m.failed(ctx, email, ErrInvalidCredentials)
	}
	if err != nil {
		return nil, err
	}
	if !credentials.VerifyPassword(op.PasswordHash, password) {
		return nil, m.failed(ctx, email, ErrInvalidCredentials)
	}
	return op, nil
}

// decoyHash is a real password hash nobody knows the password of, hashed once.
var decoyHash = sync.OnceValue(func() string {
	h, err := credentials.HashPassword("decoy password: no operator matches")
	if err != nil {
		return ""
	}
	return h
})

// Enrolled reports whether the Operator has a confirmed TOTP.
func Enrolled(op *ent.Operator) bool {
	return op.TotpConfirmedAt != nil && op.TotpSecretEncrypted != ""
}

// Step is what the password step hands the second one: the challenge and, at first
// login, the secret to enrol.
type Step struct {
	Challenge string
	// Enrolment is set when the Operator has no confirmed TOTP: the pending secret,
	// which the first correct code confirms. It is minted once and shown unchanged by
	// every password step until confirmed, so a repeated password login can neither
	// swap it nor invalidate the one being enrolled.
	Enrolment *otpcode.Key
}

// StartSecondStep mints the challenge of an Operator that passed the password step.
// An Operator without a confirmed TOTP gets a pending secret first (the one already
// pending when there is one), so there is never a path from the password alone to a
// session.
func (m *Module) StartSecondStep(ctx context.Context, op *ent.Operator) (*Step, error) {
	if err := m.licensed(); err != nil {
		return nil, err
	}
	var key *otpcode.Key
	if !Enrolled(op) {
		var err error
		if key, op, err = m.pendingEnrolment(ctx, op); err != nil {
			return nil, err
		}
	}
	challenge, err := m.challenges.Mint(authtoken.PurposeOperatorLoginChallenge, op.ID, binding(op), challengeTTL, nil)
	if err != nil {
		return nil, err
	}
	return &Step{Challenge: challenge, Enrolment: key}, nil
}

// pendingEnrolment is the Operator's unconfirmed TOTP key, minted on first use. The
// write is conditional on no secret being stored, so two password steps racing on a
// fresh Operator agree on the winner's secret; the Operator is returned as stored.
func (m *Module) pendingEnrolment(ctx context.Context, op *ent.Operator) (*otpcode.Key, *ent.Operator, error) {
	if op.TotpSecretEncrypted == "" {
		key, err := otpcode.New(Issuer, op.Email)
		if err != nil {
			return nil, nil, err
		}
		sealed, err := m.cipher.Encrypt([]byte(key.Secret))
		if err != nil {
			return nil, nil, err
		}
		n, err := m.ent.Operator.Update().
			Where(entoperator.ID(op.ID), entoperator.Or(entoperator.TotpSecretEncryptedIsNil(), entoperator.TotpSecretEncrypted(""))).
			SetTotpSecretEncrypted(sealed).
			SetTotpLastStep(0).
			Save(ctx)
		if err != nil {
			return nil, nil, err
		}
		if op, err = m.ent.Operator.Get(ctx, op.ID); err != nil {
			return nil, nil, err
		}
		if n == 1 {
			return key, op, nil
		}
	}
	raw, err := m.cipher.Decrypt(op.TotpSecretEncrypted)
	if err != nil {
		return nil, nil, err
	}
	key, err := otpcode.FromSecret(Issuer, op.Email, string(raw))
	return key, op, err
}

// binding is the value a login challenge is keyed to: it changes on every accepted
// code, session epoch bump, password change and TOTP enrolment or reset, which makes
// the challenge single-use with no store of spent ones.
func binding(op *ent.Operator) string {
	return fmt.Sprintf("%d|%d|%t|%s|%s", op.SessionEpoch, op.TotpLastStep, Enrolled(op), op.PasswordHash, op.TotpSecretEncrypted)
}

// Complete is the second login step: it checks the challenge and a current TOTP code
// and returns the Operator to start a session for. At first login the code also
// confirms the enrolment (and bumps the session epoch). The binding is re-checked
// inside the transaction with the Operator row locked, so two steps racing on one
// challenge cannot both succeed. ErrInvalidChallenge or ErrInvalidCode otherwise. While
// the Operator's failure delay runs it answers a *ThrottledError even for the right
// code; a wrong code feeds the counter, a started session resets it.
func (m *Module) Complete(ctx context.Context, challenge, code string) (*ent.Operator, error) {
	if err := m.licensed(); err != nil {
		return nil, err
	}
	var (
		held    string
		email   string
		loadErr error
	)
	id, _, err := m.challenges.Parse(challenge, authtoken.PurposeOperatorLoginChallenge, func(id int64) (string, error) {
		var op *ent.Operator
		if op, loadErr = m.ent.Operator.Get(ctx, id); loadErr != nil {
			return "", loadErr
		}
		held, email = binding(op), op.Email
		return held, nil
	})
	if loadErr != nil && !ent.IsNotFound(loadErr) {
		return nil, loadErr
	}
	if err != nil {
		return nil, ErrInvalidChallenge
	}

	if err := m.throttle(ctx, email); err != nil {
		return nil, err
	}

	tx, err := m.ent.Tx(ctx)
	if err != nil {
		return nil, err
	}
	op, err := m.verify(ctx, tx, id, held, code)
	if err != nil {
		_ = tx.Rollback()
		// A wrong code changes nothing, so rolling back loses nothing.
		if errors.Is(err, ErrInvalidCode) {
			err = m.failed(ctx, email, err)
		}
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return op, m.attempts.RecordSuccess(ctx, loginKind, email)
}

// EndSessions ends every session of the Operator by bumping its session epoch (and
// kills any live login challenge): a token copied elsewhere stops verifying too.
func (m *Module) EndSessions(ctx context.Context, id int64) error {
	if err := m.licensed(); err != nil {
		return err
	}
	return m.ent.Operator.UpdateOneID(id).AddSessionEpoch(1).Exec(ctx)
}

// ResetTOTP is the only way a lost TOTP is reset (`sphericon operator reset-totp`, no
// web flow exists): it clears the secret and its confirmation, bumps the session epoch
// (which ends every session and kills any live challenge) and leaves the Operator to
// re-enrol at next login. ErrNotFound for an unknown email.
func (m *Module) ResetTOTP(ctx context.Context, email string) error {
	if err := m.licensed(); err != nil {
		return err
	}
	n, err := m.ent.Operator.Update().
		Where(entoperator.Email(normalizeEmail(email))).
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

func (m *Module) verify(ctx context.Context, tx *ent.Tx, id int64, held, code string) (*ent.Operator, error) {
	q := tx.Operator.Query().Where(entoperator.ID(id))
	q.Modify(func(s *sql.Selector) { s.ForUpdate() })
	op, err := q.Only(ctx)
	if err != nil {
		return nil, err
	}
	if op.TotpSecretEncrypted == "" || binding(op) != held {
		return nil, ErrInvalidChallenge
	}
	raw, err := m.cipher.Decrypt(op.TotpSecretEncrypted)
	if err != nil {
		return nil, err
	}
	step, ok, err := otpcode.Match(string(raw), code, m.now(), op.TotpLastStep)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrInvalidCode
	}
	upd := tx.Operator.UpdateOneID(id).SetTotpLastStep(step)
	if !Enrolled(op) {
		upd = upd.SetTotpConfirmedAt(m.now()).AddSessionEpoch(1)
	}
	return upd.Save(ctx)
}
