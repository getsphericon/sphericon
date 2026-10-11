// Package secondfactor is a User's TOTP Second factor and Recovery codes (ADR 0020).
// A User enrolls in two phases: a pending secret is created, then confirmed with a
// valid code before it counts. The secret is sealed with the instance cipher; a
// TOTP code is single-use (the last accepted time step is stored) and checked
// against an injected clock. Recovery codes are stored hashed, each single-use, and
// shown once. Enrolling, regenerating, disabling and resetting bump the User's
// session epoch inside the change's transaction.
//
// The Second factor belongs to the User, not a Workspace, so this package works on
// the raw *ent.Client (the User is reached by id).
package secondfactor

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"

	"github.com/mokevnin/sphericon/ent"
	"github.com/mokevnin/sphericon/ent/recoverycode"
	"github.com/mokevnin/sphericon/ent/user"
	"github.com/mokevnin/sphericon/internal/accounts"
	"github.com/mokevnin/sphericon/internal/credentials"
	"github.com/mokevnin/sphericon/internal/events"
	"github.com/mokevnin/sphericon/internal/otpcode"
	"github.com/mokevnin/sphericon/internal/secrets"
)

// Issuer names the instance in the authenticator app.
const Issuer = "sphericon"

// RecoveryCodeCount is the size of a Recovery code set.
const RecoveryCodeCount = 10

var (
	// ErrInvalidCode: the code is not a current, unused TOTP code nor an unused
	// Recovery code of the User.
	ErrInvalidCode = errors.New("secondfactor: invalid code")
	// ErrNotActive: the User has no confirmed Second factor.
	ErrNotActive = errors.New("secondfactor: no active second factor")
	// ErrAlreadyActive: enrollment is refused while a Second factor is active.
	ErrAlreadyActive = errors.New("secondfactor: second factor already active")
	// ErrNoPending: there is no pending enrollment to confirm.
	ErrNoPending = errors.New("secondfactor: no pending enrollment")
	// ErrBindingMoved: the challenge binding a second step was started under has
	// moved (another second step, an epoch bump or a password change got there
	// first), so the challenge is spent.
	ErrBindingMoved = errors.New("secondfactor: challenge binding moved")
)

// Method is how a code was verified.
type Method string

const (
	MethodTOTP         Method = "totp"
	MethodRecoveryCode Method = "recovery_code"
)

// Module enrolls, verifies and removes Second factors.
type Module struct {
	ent    *ent.Client
	bus    *events.Bus
	cipher *secrets.Cipher
	now    func() time.Time
}

// New builds the module. now is the clock TOTP codes are checked against (nil
// means time.Now); tests inject one.
func New(client *ent.Client, bus *events.Bus, cipher *secrets.Cipher, now func() time.Time) *Module {
	if now == nil {
		now = time.Now
	}
	return &Module{ent: client, bus: bus, cipher: cipher, now: now}
}

// Active reports whether the User has a confirmed Second factor. A pending
// enrollment is not one.
func Active(u *ent.User) bool {
	return u.SecondFactorConfirmedAt != nil && u.SecondFactorSecretEncrypted != ""
}

// Status is what the User may know about their Second factor.
type Status struct {
	Enabled                bool
	Pending                bool
	RecoveryCodesRemaining int
}

// Status reads the User's Second factor state.
func (m *Module) Status(ctx context.Context, userID int64) (Status, error) {
	u, err := m.ent.User.Get(ctx, userID)
	if err != nil {
		return Status{}, err
	}
	if !Active(u) {
		return Status{Pending: u.SecondFactorSecretEncrypted != ""}, nil
	}
	n, err := unusedRecoveryCodes(ctx, m.ent, userID)
	if err != nil {
		return Status{}, err
	}
	return Status{Enabled: true, RecoveryCodesRemaining: n}, nil
}

// Enrollment is a pending secret, shown so the User can add it to an app.
type Enrollment struct {
	Secret string // base32 key
	URI    string // otpauth:// URI
	QRCode string // PNG data URI of the URI
}

// StartEnrollment creates a pending secret for the User, replacing an earlier
// pending one. ErrAlreadyActive when a Second factor is active: replacing it takes
// disabling it first (password and a current code).
func (m *Module) StartEnrollment(ctx context.Context, userID int64) (*Enrollment, error) {
	u, err := m.ent.User.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	if Active(u) {
		return nil, ErrAlreadyActive
	}
	key, err := otpcode.New(Issuer, u.Email)
	if err != nil {
		return nil, err
	}
	sealed, err := m.cipher.Encrypt([]byte(key.Secret))
	if err != nil {
		return nil, err
	}
	if err := m.ent.User.UpdateOneID(userID).
		SetSecondFactorSecretEncrypted(sealed).
		ClearSecondFactorConfirmedAt().
		SetSecondFactorLastStep(0).
		Exec(ctx); err != nil {
		return nil, err
	}
	return &Enrollment{Secret: key.Secret, URI: key.URI, QRCode: key.QRCode}, nil
}

// ConfirmEnrollment activates the pending secret when code is a current code for
// it. It issues the Recovery codes (returned once), bumps the session epoch and
// records `user.second_factor_enroll`, all in one transaction, and returns the
// stored User for reissuing the acting session. ErrNoPending without a pending
// enrollment, ErrInvalidCode on a wrong code.
func (m *Module) ConfirmEnrollment(ctx context.Context, userID int64, code string) (*ent.User, []string, error) {
	var (
		saved *ent.User
		codes []string
	)
	err := m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		u, err := tx.User.Get(ctx, userID)
		if err != nil {
			return err
		}
		if Active(u) {
			return ErrAlreadyActive
		}
		if u.SecondFactorSecretEncrypted == "" {
			return ErrNoPending
		}
		if err := m.useTOTP(ctx, tx, u, code); err != nil {
			return err
		}
		u, err = tx.User.UpdateOneID(userID).SetSecondFactorConfirmedAt(m.now()).AddSessionEpoch(1).Save(ctx)
		if err != nil {
			return err
		}
		if codes, err = replaceRecoveryCodes(ctx, tx, userID); err != nil {
			return err
		}
		saved = u
		return accounts.RecordUserAction(ctx, tx, pub, u, events.ActionUserSecondFactorEnroll, nil)
	})
	if err != nil {
		return nil, nil, err
	}
	return saved, codes, nil
}

// RegenerateRecoveryCodes replaces the User's Recovery codes with a fresh set
// (returned once; the previous set stops working), bumps the session epoch and
// records `user.recovery_codes_regenerate`. ErrNotActive without a Second factor.
func (m *Module) RegenerateRecoveryCodes(ctx context.Context, userID int64) (*ent.User, []string, error) {
	var (
		saved *ent.User
		codes []string
	)
	err := m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		u, err := tx.User.Get(ctx, userID)
		if err != nil {
			return err
		}
		if !Active(u) {
			return ErrNotActive
		}
		if codes, err = replaceRecoveryCodes(ctx, tx, userID); err != nil {
			return err
		}
		if u, err = tx.User.UpdateOneID(userID).AddSessionEpoch(1).Save(ctx); err != nil {
			return err
		}
		saved = u
		return accounts.RecordUserAction(ctx, tx, pub, u, events.ActionUserRecoveryCodesRegenerate, nil)
	})
	if err != nil {
		return nil, nil, err
	}
	return saved, codes, nil
}

// Disable removes the User's Second factor when code is a current TOTP code or an
// unused Recovery code (the caller has checked the password). It clears the factor
// and its Recovery codes, bumps the session epoch and records
// `user.second_factor_disable`. ErrNotActive without a Second factor,
// ErrInvalidCode on a wrong code.
func (m *Module) Disable(ctx context.Context, userID int64, code string) (*ent.User, error) {
	var saved *ent.User
	err := m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		u, err := tx.User.Get(ctx, userID)
		if err != nil {
			return err
		}
		if !Active(u) {
			return ErrNotActive
		}
		if _, err := m.verify(ctx, tx, pub, u, code); err != nil {
			return err
		}
		if saved, err = remove(ctx, tx, userID); err != nil {
			return err
		}
		return accounts.RecordUserAction(ctx, tx, pub, saved, events.ActionUserSecondFactorDisable, nil)
	})
	return saved, err
}

// Verify checks a second-step code for the User at the module's clock: a current
// TOTP code (each time step works once) or an unused Recovery code (spent here,
// and recorded as `user.recovery_code_use`). It returns which one matched.
//
// binding is the ChallengeBinding the step's challenge was minted under. It is
// re-checked inside the transaction with the User row locked, so of two second
// steps racing on one challenge (say one with a TOTP code, one with a Recovery
// code) the later sees the binding the earlier moved and fails: a challenge starts
// at most one session. ErrNotActive without a Second factor, ErrBindingMoved when
// the binding moved, ErrInvalidCode otherwise.
func (m *Module) Verify(ctx context.Context, userID int64, binding, code string) (Method, error) {
	var method Method
	err := m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		q := tx.User.Query().Where(user.ID(userID))
		q.Modify(func(s *sql.Selector) { s.ForUpdate() })
		u, err := q.Only(ctx)
		if err != nil {
			return err
		}
		if !Active(u) {
			return ErrNotActive
		}
		current, err := challengeBinding(ctx, tx, u)
		if err != nil {
			return err
		}
		if subtle.ConstantTimeCompare([]byte(current), []byte(binding)) != 1 {
			return ErrBindingMoved
		}
		method, err = m.verify(ctx, tx, pub, u, code)
		return err
	})
	return method, err
}

// ChallengeBinding is the value a login challenge (ADR 0020) is keyed to: it
// changes on every successful Verify (a TOTP success moves the last accepted time
// step, a Recovery code success lowers the count of unused codes), on a password
// change and on every session epoch bump (enroll, regenerate, disable, reset).
// Keying the challenge's signature to it makes the challenge single-use without a
// store of spent challenges, so it holds across instances; Verify re-checks it
// under a lock, so two racing second steps cannot both use it. ErrNotActive
// without a Second factor.
func (m *Module) ChallengeBinding(ctx context.Context, u *ent.User) (string, error) {
	if !Active(u) {
		return "", ErrNotActive
	}
	return challengeBinding(ctx, m.ent, u)
}

func challengeBinding(ctx context.Context, client *ent.Client, u *ent.User) (string, error) {
	unused, err := unusedRecoveryCodes(ctx, client, u.ID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d|%d|%d|%s", u.SessionEpoch, u.SecondFactorLastStep, unused, u.PasswordHash), nil
}

// unusedRecoveryCodes counts the User's Recovery codes not spent yet.
func unusedRecoveryCodes(ctx context.Context, client *ent.Client, userID int64) (int, error) {
	return client.RecoveryCode.Query().
		Where(recoverycode.UserID(userID), recoverycode.UsedAtIsNil()).
		Count(ctx)
}

// Reset clears the User's Second factor and Recovery codes and bumps the session
// epoch, ending every session of theirs: an Owner or Admin resetting a member of
// workspaceID. It records `user.second_factor_reset` by actor in that Workspace
// only. ErrNotActive without a Second factor (nothing changes).
func (m *Module) Reset(ctx context.Context, userID int64, actor events.Actor, workspaceID int64) error {
	return m.reset(ctx, userID, func(pub events.Publisher, _ *ent.Client, u *ent.User) error {
		return accounts.RecordActionOnUserIn(ctx, pub, actor, u, events.ActionUserSecondFactorReset, nil, workspaceID)
	})
}

// ResetByOperator is Reset from the operator command: the platform Operator `by`
// (shown to customers as "sphericon staff") is recorded in every Workspace the User
// holds a Membership in.
func (m *Module) ResetByOperator(ctx context.Context, userID int64, by string) error {
	actor := events.Actor{Kind: events.ActorOperator, ID: by}
	return m.reset(ctx, userID, func(pub events.Publisher, tx *ent.Client, u *ent.User) error {
		return accounts.RecordActionOnUser(ctx, tx, pub, actor, u, events.ActionUserSecondFactorReset, nil)
	})
}

func (m *Module) reset(ctx context.Context, userID int64, record func(events.Publisher, *ent.Client, *ent.User) error) error {
	return m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		u, err := tx.User.Get(ctx, userID)
		if err != nil {
			return err
		}
		if !Active(u) {
			return ErrNotActive
		}
		if u, err = remove(ctx, tx, userID); err != nil {
			return err
		}
		return record(pub, tx, u)
	})
}

func (m *Module) verify(ctx context.Context, tx *ent.Client, pub events.Publisher, u *ent.User, code string) (Method, error) {
	if otpcode.IsCode(code) {
		if err := m.useTOTP(ctx, tx, u, code); err != nil {
			return "", err
		}
		return MethodTOTP, nil
	}
	n, err := tx.RecoveryCode.Update().
		Where(
			recoverycode.UserID(u.ID),
			recoverycode.CodeHash(credentials.HashRecoveryCode(code)),
			recoverycode.UsedAtIsNil(),
		).
		SetUsedAt(m.now()).
		Save(ctx)
	if err != nil {
		return "", err
	}
	if n != 1 {
		return "", ErrInvalidCode
	}
	return MethodRecoveryCode, accounts.RecordUserAction(ctx, tx, pub, u, events.ActionUserRecoveryCodeUse, nil)
}

// useTOTP accepts code when it is the User's code for a time step within the skew
// of now and later than the last step accepted, and stores that step, so the code
// cannot be replayed. The conditional write makes two concurrent uses of one code
// accept only one.
func (m *Module) useTOTP(ctx context.Context, tx *ent.Client, u *ent.User, code string) error {
	raw, err := m.cipher.Decrypt(u.SecondFactorSecretEncrypted)
	if err != nil {
		return err
	}
	step, ok, err := otpcode.Match(string(raw), code, m.now(), u.SecondFactorLastStep)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidCode
	}
	n, err := tx.User.Update().
		Where(user.ID(u.ID), user.SecondFactorLastStepLT(step)).
		SetSecondFactorLastStep(step).
		Save(ctx)
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrInvalidCode
	}
	return nil
}

// remove clears the Second factor and the Recovery codes and bumps the epoch.
func remove(ctx context.Context, tx *ent.Client, userID int64) (*ent.User, error) {
	if _, err := tx.RecoveryCode.Delete().Where(recoverycode.UserID(userID)).Exec(ctx); err != nil {
		return nil, err
	}
	return tx.User.UpdateOneID(userID).
		ClearSecondFactorSecretEncrypted().
		ClearSecondFactorConfirmedAt().
		SetSecondFactorLastStep(0).
		AddSessionEpoch(1).
		Save(ctx)
}

// replaceRecoveryCodes deletes the User's Recovery codes and stores a fresh set,
// returning its plaintext.
func replaceRecoveryCodes(ctx context.Context, tx *ent.Client, userID int64) ([]string, error) {
	if _, err := tx.RecoveryCode.Delete().Where(recoverycode.UserID(userID)).Exec(ctx); err != nil {
		return nil, err
	}
	codes := make([]string, RecoveryCodeCount)
	builders := make([]*ent.RecoveryCodeCreate, RecoveryCodeCount)
	for i := range codes {
		codes[i] = newRecoveryCode()
		builders[i] = tx.RecoveryCode.Create().SetUserID(userID).SetCodeHash(credentials.HashRecoveryCode(codes[i]))
	}
	if err := tx.RecoveryCode.CreateBulk(builders...).Exec(ctx); err != nil {
		return nil, err
	}
	return codes, nil
}

// newRecoveryCode is ten random base32 characters (50 bits) as two groups of five.
func newRecoveryCode() string {
	s := strings.ToLower(rand.Text()[:10])
	return s[:5] + "-" + s[5:]
}
