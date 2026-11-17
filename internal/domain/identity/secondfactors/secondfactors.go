// Package secondfactors owns a User's TOTP Second factor and its Recovery codes (GLOSSARY, ADR
// 0020, ADR 0110): the factor's row (its secret, its confirmation, the last accepted step) and
// the codes, and the Second factor check that comes before every change of them: enrolling,
// confirming, verifying a code, regenerating the codes and disabling, and the Second factor reset
// (Reset, ResetByStaff). It reads the factor for everyone who needs it (the login, the sign-in,
// the notices, the re-encryption), and owns the Second factor deadline of a Workspace that
// requires one: the grace, its reminders and its part of the admission of a request
// (CheckRequirement).
//
// A User enrolls in two phases: a pending secret is created, then confirmed with a valid code
// before it counts. The secret is sealed with the instance cipher; a TOTP code is single-use (the
// last accepted time step is stored) and checked against an injected clock. Recovery codes are
// stored hashed, each single-use, and shown once. Confirming, regenerating, disabling and
// resetting bump the User's session epoch inside the change's transaction, so the module sits
// above internal/domain/identity/users: it reads the User's session epoch and password hash and
// bumps the epoch through it. A Second factor reset also removes the User's Passkeys
// (passkeys.Module.RemoveAllIn): internal/domain/identity/passkeys sits beside this module in identity,
// so the reset is this module's operation (ADR 0112), checked here and written by its lifecycle
// like every other change of the factor.
//
// The Second factor belongs to the User, not a Workspace, so there is no *ent.Scoped to take:
// the module holds the raw *ent.Client (the User is reached by id), one of the closed list of
// docs/architecture/workspace-scoping.md (Workspace scoping).
package secondfactors

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/secondfactor"
	"github.com/getsphericon/sphericon/internal/domain/identity/login"
	"github.com/getsphericon/sphericon/internal/domain/identity/passkeys"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
	"github.com/getsphericon/sphericon/internal/kernel/totp"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

var (
	// ErrInvalidCode: the code is not a current, unused TOTP code nor an unused
	// Recovery code of the User. It refuses the `code` field of the User's own steps
	// (confirming an enrollment, disabling); the login adapter answers it as the login
	// module's ErrInvalidCode.
	ErrInvalidCode = validation.RefuseField(fieldname.Code, "secondfactor: invalid code", i18n.ErrorsSecondFactorCodeInvalid)
	// ErrNotActive: the User has no confirmed Second factor, so their own step that needs one
	// (regenerating the Recovery codes, disabling) has nothing to act on.
	ErrNotActive = validation.Conflict("secondfactor: no active second factor", i18n.ErrorsSecondFactorNotActive)
	// ErrDeletionCodeInvalid: the code that confirms the User's own deletion is wrong. Like a
	// wrong password it fails the proof of who asks (403), and the caller counts it on the
	// Login throttle (ADR 0025).
	ErrDeletionCodeInvalid = validation.Denied("secondfactor: invalid deletion code", i18n.ErrorsSecondFactorCodeInvalid)
	// ErrAlreadyActive: enrollment is refused while a Second factor is active.
	ErrAlreadyActive = validation.Conflict("secondfactor: second factor already active", i18n.ErrorsSecondFactorAlreadyActive)
	// ErrNoPending: there is no pending enrollment to confirm.
	ErrNoPending = validation.Conflict("secondfactor: no pending enrollment", i18n.ErrorsSecondFactorNoPending)
	// ErrBindingMoved: the challenge binding a second step was started under has
	// moved (another second step, an epoch bump or a password change got there
	// first), so the challenge is spent.
	ErrBindingMoved = validation.Unauthorized("secondfactor: challenge binding moved", i18n.ErrorsLoginChallengeInvalid)
)

// Method is how a code was verified.
type Method string

const (
	MethodTOTP         Method = "totp"
	MethodRecoveryCode Method = "recovery_code"
)

// Users is what the module asks of the User (users.Module): the proof of identity the User's own
// changes of their factor take, the lock and the session epoch of the User's row a change moves,
// and the Audit entries of the change, in the transaction of the change.
type Users interface {
	ProveIdentity(ctx context.Context, u *ent.User, proof users.IdentityProof) error
	UserByEmailIn(ctx context.Context, tx *ent.Client, email string) (*ent.User, error)
	LockIn(ctx context.Context, tx *ent.Client, userID int64) (*ent.User, error)
	BumpSessionEpochFromIn(ctx context.Context, tx *ent.Client, before *ent.User) (*ent.User, error)
	RecordUserAction(ctx context.Context, tx *ent.Client, in users.UserAction) error
	RecordActionOnUser(ctx context.Context, tx *ent.Client, in users.ActionOnUser) error
	RecordActionOnUserIn(ctx context.Context, ts *ent.Scoped, in users.ActionOnUser) error
}

// Module enrolls, verifies and removes Second factors, and reads them.
type Module struct {
	ent      *ent.Client
	bus      *events.Bus
	cipher   *secrets.Cipher
	users    Users
	passkeys *passkeys.Module
	now      func() time.Time
}

// New builds the module. u proves the identity of the User's own steps; keys are the Passkeys a
// Second factor reset removes; now is the Instance clock TOTP codes, the Two-factor grace and its
// reminders are checked against.
func New(client *ent.Client, bus *events.Bus, cipher *secrets.Cipher, u Users, keys *passkeys.Module, now func() time.Time) *Module {
	return &Module{ent: client, bus: bus, cipher: cipher, users: u, passkeys: keys, now: now}
}

// factorOf reads the User's factor row on client, nil when the User never enrolled.
func factorOf(ctx context.Context, client *ent.Client, userID int64) (*ent.SecondFactor, error) {
	f, err := client.SecondFactor.Query().Where(secondfactor.UserID(userID)).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	return f, err
}

// snapshotIn reads what a change checks on the transaction tx: the User and its factor row.
func snapshotIn(ctx context.Context, tx *ent.Client, userID int64) (snapshot, error) {
	u, err := tx.User.Get(ctx, userID)
	if err != nil {
		return snapshot{}, err
	}
	f, err := factorOf(ctx, tx, userID)
	return snapshot{User: u, Factor: f}, err
}

// Active reports whether the User has a confirmed Second factor. A pending enrollment is not one.
func (m *Module) Active(ctx context.Context, userID int64) (bool, error) {
	f, err := factorOf(ctx, m.ent, userID)
	return active(f), err
}

// CodesIssued is a change that issued a fresh set of Recovery codes: the stored User, whose
// session the caller reissues (the epoch moved), and the codes, shown once.
type CodesIssued struct {
	User          *ent.User
	RecoveryCodes []string
}

// Status is what the User may know about their Second factor.
type Status struct {
	Enabled                bool
	Pending                bool
	RecoveryCodesRemaining int
}

// Status reads the User's Second factor state.
func (m *Module) Status(ctx context.Context, userID int64) (Status, error) {
	f, err := factorOf(ctx, m.ent, userID)
	if err != nil {
		return Status{}, err
	}
	if !active(f) {
		return Status{Pending: stateOf(f) == factorPending}, nil
	}
	n, err := unusedCodes(ctx, m.ent, userID)
	if err != nil {
		return Status{}, err
	}
	return Status{Enabled: true, RecoveryCodesRemaining: n}, nil
}

// Enrollment is a pending secret, shown so the User can add it to an app.
type Enrollment struct {
	ID     int64  // identity of the pending enrollment
	Secret string // base32 key
	URI    string // otpauth:// URI
	QRCode string // PNG data URI of the URI
}

// StartEnrollment creates a pending secret for the User u once they prove their identity
// (users.Module.ProveIdentity: a hijacked session alone cannot enroll a factor of its own),
// replacing an earlier pending one. ErrAlreadyActive when a Second factor is active: replacing it
// takes disabling it first (password and a current code).
func (m *Module) StartEnrollment(ctx context.Context, u *ent.User, proof users.IdentityProof) (*Enrollment, error) {
	if err := m.users.ProveIdentity(ctx, u, proof); err != nil {
		return nil, err
	}
	userID := u.ID
	var enrollment *Enrollment
	err := m.bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		before, err := snapshotIn(ctx, tx, userID)
		if err != nil {
			return err
		}
		if active(before.Factor) {
			return ErrAlreadyActive
		}
		key, err := totp.New(users.Issuer, before.User.Email)
		if err != nil {
			return err
		}
		sealed, err := m.cipher.Encrypt([]byte(key.Secret))
		if err != nil {
			return err
		}
		moved, err := m.move(ctx, tx, before, change{trigger: factorStart, sealedSecret: sealed, at: m.now()})
		if err != nil {
			return err
		}
		enrollment = &Enrollment{ID: moved.Factor.Generation, Secret: key.Secret, URI: key.URI, QRCode: key.QRCode}
		return nil
	})
	return enrollment, err
}

// ConfirmInput binds the code to the enrollment whose secret the caller received, with the proof
// of identity the confirmation takes.
type ConfirmInput struct {
	EnrollmentID int64
	Code         string
	Proof        users.IdentityProof
}

// ConfirmEnrollment activates the pending secret of the User u, once they prove their identity
// with in.Proof (users.Module.ProveIdentity), when in.Code is a current code for it. It issues the Recovery codes (returned once), bumps the session epoch and
// records `user.second_factor_enroll`, all in one transaction, and returns the
// stored User for reissuing the acting session with the codes. ErrNoPending without a pending
// enrollment, ErrInvalidCode on a wrong code.
func (m *Module) ConfirmEnrollment(ctx context.Context, u *ent.User, in ConfirmInput) (*CodesIssued, error) {
	if err := m.users.ProveIdentity(ctx, u, in.Proof); err != nil {
		return nil, err
	}
	userID := u.ID
	var issued *CodesIssued
	err := m.bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		before, err := snapshotIn(ctx, tx, userID)
		if err != nil {
			return err
		}
		switch stateOf(before.Factor) {
		case factorActive:
			return ErrAlreadyActive
		case factorNone:
			return ErrNoPending
		case factorPending:
		}
		if before.Factor.Generation != in.EnrollmentID {
			return lifecycle.ErrInvalidTransition
		}
		step, err := m.totpStep(before.Factor, in.Code)
		if err != nil {
			return err
		}
		moved, err := m.move(ctx, tx, before, change{trigger: factorConfirm, step: step, at: m.now()})
		if err != nil {
			return err
		}
		codes, err := replaceCodesIn(ctx, tx, userID)
		if err != nil {
			return err
		}
		issued = &CodesIssued{User: moved.User, RecoveryCodes: codes}
		return m.users.RecordUserAction(ctx, tx, users.UserAction{User: moved.User, Action: events.ActionUserSecondFactorEnroll})
	})
	return issued, err
}

// RegenerateRecoveryCodes replaces the Recovery codes of the User u, once they prove their identity
// (users.Module.ProveIdentity), with a fresh set (returned once; the previous set stops working), bumps the session epoch and
// records `user.recovery_codes_regenerate`. ErrNotActive without a Second factor.
func (m *Module) RegenerateRecoveryCodes(ctx context.Context, u *ent.User, proof users.IdentityProof) (*CodesIssued, error) {
	if err := m.users.ProveIdentity(ctx, u, proof); err != nil {
		return nil, err
	}
	userID := u.ID
	var issued *CodesIssued
	err := m.bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		before, err := snapshotIn(ctx, tx, userID)
		if err != nil {
			return err
		}
		if !active(before.Factor) {
			return ErrNotActive
		}
		moved, err := m.move(ctx, tx, before, change{trigger: factorRegenerate, at: m.now()})
		if err != nil {
			return err
		}
		codes, err := replaceCodesIn(ctx, tx, userID)
		if err != nil {
			return err
		}
		issued = &CodesIssued{User: moved.User, RecoveryCodes: codes}
		return m.users.RecordUserAction(ctx, tx, users.UserAction{User: moved.User, Action: events.ActionUserRecoveryCodesRegenerate})
	})
	return issued, err
}

// DisableInput is the code that confirms disabling the Second factor, with the proof of identity it
// takes.
type DisableInput struct {
	Code  string
	Proof users.IdentityProof
}

// Disable removes the Second factor of the User u, once they prove their identity with in.Proof
// (users.Module.ProveIdentity), when in.Code is a current TOTP code or an unused Recovery code. It
// clears the factor
// and its Recovery codes, bumps the session epoch and records
// `user.second_factor_disable`. ErrNotActive without a Second factor,
// ErrInvalidCode on a wrong code.
func (m *Module) Disable(ctx context.Context, u *ent.User, in DisableInput) (*ent.User, error) {
	if err := m.users.ProveIdentity(ctx, u, in.Proof); err != nil {
		return nil, err
	}
	userID := u.ID
	var saved *ent.User
	err := m.bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		locked, err := m.users.LockIn(ctx, tx, userID)
		if err != nil {
			return err
		}
		f, err := factorOf(ctx, tx, userID)
		if err != nil {
			return err
		}
		if !active(f) {
			return ErrNotActive
		}
		_, verified, err := m.verify(ctx, tx, snapshot{User: locked, Factor: f}, in.Code)
		if err != nil {
			return err
		}
		removed, err := m.remove(ctx, tx, verified, factorDisable)
		if err != nil {
			return err
		}
		saved = removed.User
		return m.users.RecordUserAction(ctx, tx, users.UserAction{User: saved, Action: events.ActionUserSecondFactorDisable})
	})
	return saved, err
}

// Verify checks a second-step code for the User at the module's clock: a current
// TOTP code (each time step works once) or an unused Recovery code (spent here,
// and recorded as `user.recovery_code_use`). It returns which one matched.
//
// pending is the sign-in waiting for its second step: its User and the ChallengeBinding the
// step's challenge was minted under. The binding is
// re-checked inside the transaction with the User row locked, so of two second
// steps racing on one challenge (say one with a TOTP code, one with a Recovery
// code) the later sees the binding the earlier moved and fails: a challenge starts
// at most one session. ErrNotActive without a Second factor, ErrBindingMoved when
// the binding moved, ErrInvalidCode otherwise.
func (m *Module) Verify(ctx context.Context, pending login.Pending[*ent.User], code string) (Method, error) {
	binding := pending.Binding
	var method Method
	err := m.bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		u, err := m.users.LockIn(ctx, tx, pending.Principal.ID)
		if err != nil {
			return err
		}
		f, err := factorOf(ctx, tx, u.ID)
		if err != nil {
			return err
		}
		if !active(f) {
			return ErrNotActive
		}
		current, err := challengeBinding(ctx, tx, u, f)
		if err != nil {
			return err
		}
		if subtle.ConstantTimeCompare([]byte(current), []byte(binding)) != 1 {
			return ErrBindingMoved
		}
		method, _, err = m.verify(ctx, tx, snapshot{User: u, Factor: f}, code)
		return err
	})
	return method, err
}

// DeletionCode spends code on the transaction tx of User deletion (ADR 0050, the use case
// internal/usecase/userdeletion), a transaction of the module's bus whose User row locked the caller
// has locked and freshly read: with a Second factor the code must be a current TOTP code or an
// unused Recovery code (ErrDeletionCodeInvalid otherwise), and it is spent, its use recorded, only if
// the deletion commits; without one it confirms nothing. It keeps the lock order of every change of
// the factor: the User row, the factor row, then the codes.
func (m *Module) DeletionCode(ctx context.Context, tx *ent.Client, locked *ent.User, code string) error {
	f, err := factorOf(ctx, tx, locked.ID)
	if err != nil || !active(f) {
		return err
	}
	_, _, err = m.verify(ctx, tx, snapshot{User: locked, Factor: f}, code)
	if errors.Is(err, ErrInvalidCode) {
		return ErrDeletionCodeInvalid
	}
	return err
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
	f, err := factorOf(ctx, m.ent, u.ID)
	if err != nil {
		return "", err
	}
	if !active(f) {
		return "", ErrNotActive
	}
	return challengeBinding(ctx, m.ent, u, f)
}

func challengeBinding(ctx context.Context, client *ent.Client, u *ent.User, f *ent.SecondFactor) (string, error) {
	unused, err := unusedCodes(ctx, client, u.ID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d|%d|%d|%s", u.SessionEpoch, f.LastStep, unused, u.PasswordHash), nil
}

// verify fences the factor before touching Recovery codes, keeping the same lock order as
// enrollment, regeneration, removal and the login challenge check: the User row, the factor row,
// then the codes.
func (m *Module) verify(ctx context.Context, tx *ent.Client, before snapshot, code string) (Method, snapshot, error) {
	step := int64(0)
	if totp.IsCode(code) {
		var err error
		step, err = m.totpStep(before.Factor, code)
		if err != nil {
			return "", snapshot{}, err
		}
	}
	moved, err := m.move(ctx, tx, before, change{trigger: factorVerify, step: step, at: m.now()})
	if err != nil {
		return "", snapshot{}, err
	}
	verified := snapshot(moved)
	if step > 0 {
		return MethodTOTP, verified, nil
	}
	spent, err := spendCodeIn(ctx, tx, before.User.ID, code, m.now())
	if err != nil {
		return "", snapshot{}, err
	}
	if !spent {
		return "", snapshot{}, ErrInvalidCode
	}
	err = m.users.RecordUserAction(ctx, tx, users.UserAction{User: verified.User, Action: events.ActionUserRecoveryCodeUse})
	return MethodRecoveryCode, verified, err
}

// totpStep checks the code against this exact secret and replay step. The move consumes
// both in its single conditional write; no separate acceptance can outlive a reset.
func (m *Module) totpStep(f *ent.SecondFactor, code string) (int64, error) {
	raw, err := m.cipher.Decrypt(f.SecretEncrypted)
	if err != nil {
		return 0, err
	}
	step, ok, err := totp.Match(string(raw), code, m.now(), f.LastStep)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, ErrInvalidCode
	}
	return step, nil
}

// remove clears the factor by t (disabling it, or a Second factor reset) before its Recovery codes
// go, and bumps the epoch in the same transaction.
func (m *Module) remove(ctx context.Context, tx *ent.Client, before snapshot, t trigger) (outcome, error) {
	moved, err := m.move(ctx, tx, before, change{trigger: t})
	if err != nil {
		return outcome{}, err
	}
	if err := deleteCodesIn(ctx, tx, before.User.ID); err != nil {
		return outcome{}, err
	}
	return moved, nil
}
