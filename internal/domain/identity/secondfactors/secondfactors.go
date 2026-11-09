// Package secondfactors owns a User's TOTP Second factor and its Recovery codes (GLOSSARY, ADR
// 0020, ADR 0110): the factor's row (its secret, its confirmation, the last accepted step) and
// the codes, and the Second factor check that comes before every change of them: enrolling,
// confirming, verifying a code, regenerating the codes and disabling. It reads the factor for
// everyone who needs it (the login, the sign-in, the notices, the re-encryption), and owns the
// Second factor deadline of a Workspace that requires one: the grace, its reminders and its part
// of the admission of a request (Admit).
//
// A User enrolls in two phases: a pending secret is created, then confirmed with a valid code
// before it counts. The secret is sealed with the instance cipher; a TOTP code is single-use (the
// last accepted time step is stored) and checked against an injected clock. Recovery codes are
// stored hashed, each single-use, and shown once. Confirming, regenerating, disabling and
// resetting bump the User's session epoch inside the change's transaction, so the module sits
// above internal/domain/identity/users: it reads the User's session epoch and password hash and
// bumps the epoch through it. A Second factor reset also removes the User's Passkeys, so it is a
// use case (internal/usecase/secondfactorreset) over the unchecked secondfactorsunchecked.ResetIn
// (ADR 0110), which writes the reset move of this module's lifecycle (Machine).
//
// The Second factor belongs to the User, not a Workspace, so there is no *ent.Scoped to take:
// the module holds the raw *ent.Client (the User is reached by id), one of the closed list of
// docs/architecture.md (Workspace scoping).
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
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/infra/events"
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
	ErrInvalidCode = validation.RefuseField("code", "secondfactor: invalid code", i18n.ErrorsSecondFactorCodeInvalid)
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

// Users is what the module asks of the User: their part of the admission of a request, which a
// required acceptance pending refuses (users.Module.CheckAcceptance), and the proof of identity the
// User's own changes of their factor take (users.Module.ProveIdentity).
type Users interface {
	CheckAcceptance(ctx context.Context, userID int64) error
	ProveIdentity(ctx context.Context, u *ent.User, proof users.IdentityProof) error
}

// Module enrolls, verifies and removes Second factors, and reads them.
type Module struct {
	ent    *ent.Client
	bus    *events.Bus
	cipher *secrets.Cipher
	users  Users
	now    func() time.Time
}

// New builds the module. u is the User's part of Admit and the proof of identity of the User's
// own steps; now is the Instance clock TOTP codes and the Two-factor grace are checked against.
func New(client *ent.Client, bus *events.Bus, cipher *secrets.Cipher, u Users, now func() time.Time) *Module {
	return &Module{ent: client, bus: bus, cipher: cipher, users: u, now: now}
}

// factorOf reads the User's factor row on client, nil when the User never enrolled.
func factorOf(ctx context.Context, client *ent.Client, userID int64) (*ent.SecondFactor, error) {
	f, err := client.SecondFactor.Query().Where(secondfactor.UserID(userID)).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	return f, err
}

// SnapshotIn reads what a change checks on the transaction tx: the User and its factor row.
func SnapshotIn(ctx context.Context, tx *ent.Client, userID int64) (Snapshot, error) {
	u, err := tx.User.Get(ctx, userID)
	if err != nil {
		return Snapshot{}, err
	}
	f, err := factorOf(ctx, tx, userID)
	return Snapshot{User: u, Factor: f}, err
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
		return Status{Pending: stateOf(f) == FactorPending}, nil
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
		before, err := SnapshotIn(ctx, tx, userID)
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
		moved, err := move(ctx, tx, before, Change{Trigger: FactorStart, SealedSecret: sealed, At: m.now()})
		if err != nil {
			return err
		}
		enrollment = &Enrollment{ID: moved.Factor.Generation, Secret: key.Secret, URI: key.URI, QRCode: key.QRCode}
		return nil
	})
	return enrollment, err
}

// ConfirmInput binds the code to the enrollment whose secret the caller received.
type ConfirmInput struct {
	EnrollmentID int64
	Code         string
}

// ConfirmEnrollment activates the pending secret of the User u, once they prove their identity
// (users.Module.ProveIdentity), when code is a current code for it. It issues the Recovery codes (returned once), bumps the session epoch and
// records `user.second_factor_enroll`, all in one transaction, and returns the
// stored User for reissuing the acting session with the codes. ErrNoPending without a pending
// enrollment, ErrInvalidCode on a wrong code.
func (m *Module) ConfirmEnrollment(ctx context.Context, u *ent.User, proof users.IdentityProof, in ConfirmInput) (*CodesIssued, error) {
	if err := m.users.ProveIdentity(ctx, u, proof); err != nil {
		return nil, err
	}
	userID := u.ID
	var issued *CodesIssued
	err := m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		before, err := SnapshotIn(ctx, tx, userID)
		if err != nil {
			return err
		}
		switch stateOf(before.Factor) {
		case FactorActive:
			return ErrAlreadyActive
		case FactorNone:
			return ErrNoPending
		case FactorPending:
		}
		if before.Factor.Generation != in.EnrollmentID {
			return lifecycle.ErrInvalidTransition
		}
		step, err := m.totpStep(before.Factor, in.Code)
		if err != nil {
			return err
		}
		moved, err := move(ctx, tx, before, Change{Trigger: FactorConfirm, Step: step, At: m.now()})
		if err != nil {
			return err
		}
		codes, err := replaceCodesIn(ctx, tx, userID)
		if err != nil {
			return err
		}
		issued = &CodesIssued{User: moved.User, RecoveryCodes: codes}
		return users.RecordUserAction(ctx, tx, pub, moved.User, events.ActionUserSecondFactorEnroll, nil)
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
	err := m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		before, err := SnapshotIn(ctx, tx, userID)
		if err != nil {
			return err
		}
		if !active(before.Factor) {
			return ErrNotActive
		}
		moved, err := move(ctx, tx, before, Change{Trigger: FactorRegenerate, At: m.now()})
		if err != nil {
			return err
		}
		codes, err := replaceCodesIn(ctx, tx, userID)
		if err != nil {
			return err
		}
		issued = &CodesIssued{User: moved.User, RecoveryCodes: codes}
		return users.RecordUserAction(ctx, tx, pub, moved.User, events.ActionUserRecoveryCodesRegenerate, nil)
	})
	return issued, err
}

// Disable removes the Second factor of the User u, once they prove their identity
// (users.Module.ProveIdentity), when code is a current TOTP code or an unused Recovery code. It
// clears the factor
// and its Recovery codes, bumps the session epoch and records
// `user.second_factor_disable`. ErrNotActive without a Second factor,
// ErrInvalidCode on a wrong code.
func (m *Module) Disable(ctx context.Context, u *ent.User, proof users.IdentityProof, code string) (*ent.User, error) {
	if err := m.users.ProveIdentity(ctx, u, proof); err != nil {
		return nil, err
	}
	userID := u.ID
	var saved *ent.User
	err := m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		locked, err := users.LockIn(ctx, tx, userID)
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
		_, verified, err := m.verify(ctx, tx, pub, Snapshot{User: locked, Factor: f}, code)
		if err != nil {
			return err
		}
		removed, err := remove(ctx, tx, verified)
		if err != nil {
			return err
		}
		saved = removed.User
		return users.RecordUserAction(ctx, tx, pub, saved, events.ActionUserSecondFactorDisable, nil)
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
	err := m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		u, err := users.LockIn(ctx, tx, pending.Principal.ID)
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
		method, _, err = m.verify(ctx, tx, pub, Snapshot{User: u, Factor: f}, code)
		return err
	})
	return method, err
}

// DeletionCode spends code on the transaction tx of User deletion (ADR 0050, the use case
// internal/usecase/userdeletion), whose User row locked the caller has locked and freshly read:
// with a Second factor the code must be a current TOTP code or an unused Recovery code
// (ErrDeletionCodeInvalid otherwise), and it is spent only if the deletion commits; without one
// it confirms nothing. It keeps the lock order of every change of the factor: the User row, the
// factor row, then the codes.
func (m *Module) DeletionCode(ctx context.Context, tx *ent.Client, pub events.Publisher, locked *ent.User, code string) error {
	f, err := factorOf(ctx, tx, locked.ID)
	if err != nil || !active(f) {
		return err
	}
	_, _, err = m.verify(ctx, tx, pub, Snapshot{User: locked, Factor: f}, code)
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
func (m *Module) verify(ctx context.Context, tx *ent.Client, pub events.Publisher, before Snapshot, code string) (Method, Snapshot, error) {
	step := int64(0)
	if totp.IsCode(code) {
		var err error
		step, err = m.totpStep(before.Factor, code)
		if err != nil {
			return "", Snapshot{}, err
		}
	}
	moved, err := move(ctx, tx, before, Change{Trigger: FactorVerify, Step: step, At: m.now()})
	if err != nil {
		return "", Snapshot{}, err
	}
	verified := Snapshot(moved)
	if step > 0 {
		return MethodTOTP, verified, nil
	}
	spent, err := spendCodeIn(ctx, tx, before.User.ID, code, m.now())
	if err != nil {
		return "", Snapshot{}, err
	}
	if !spent {
		return "", Snapshot{}, ErrInvalidCode
	}
	err = users.RecordUserAction(ctx, tx, pub, verified.User, events.ActionUserRecoveryCodeUse, nil)
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

// remove disables the factor before its Recovery codes go, and bumps the epoch in the same
// transaction.
func remove(ctx context.Context, tx *ent.Client, before Snapshot) (Moved, error) {
	moved, err := move(ctx, tx, before, Change{Trigger: FactorDisable})
	if err != nil {
		return Moved{}, err
	}
	if err := deleteCodesIn(ctx, tx, before.User.ID); err != nil {
		return Moved{}, err
	}
	return moved, nil
}
