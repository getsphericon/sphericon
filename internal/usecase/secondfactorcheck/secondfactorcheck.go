// Package secondfactorcheck is the use case of the Second factor check (GLOSSARY, ADR 0020, ADR
// 0102): the check of a code or of the factor's state that comes before every change of a User's
// TOTP Second factor and Recovery codes, enrolling, verifying a code, regenerating the codes,
// disabling and resetting, each an operation across the User (internal/domain/identity/users, which owns the factor's columns
// and the session epoch), the Recovery codes (internal/domain/identity/recoverycodes) and, for a reset, the
// Passkeys (internal/domain/identity/passkeys). It checks the code or the state first and writes each row through
// its owner, in one transaction.
//
// A User enrolls in two phases: a pending secret is created, then confirmed with a valid code
// before it counts. The secret is sealed with the instance cipher; a TOTP code is single-use (the
// last accepted time step is stored) and checked against an injected clock. Recovery codes are
// stored hashed, each single-use, and shown once. Enrolling, regenerating, disabling and
// resetting bump the User's session epoch inside the change's transaction.
//
// The Second factor belongs to the User, not a Workspace, so there is no *ent.Scoped to take:
// this use case holds the raw *ent.Client (the User is reached by id), one of the closed list of
// docs/architecture/workspace-scoping.md (Workspace scoping).
package secondfactorcheck

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/internal/domain/identity/login"
	"github.com/getsphericon/sphericon/internal/domain/identity/passkeys"
	"github.com/getsphericon/sphericon/internal/domain/identity/recoverycodes"
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
	// ErrNothingToReset: the member has neither an active or pending Second factor nor a Passkey, so a reset has
	// nothing to clear. It refuses the `member` the reset names.
	ErrNothingToReset = validation.RefuseField("member", "secondfactor: the member has no second factor or passkey", i18n.ErrorsSecondFactorResetNothing)
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

// UseCase enrolls, verifies and removes Second factors.
type UseCase struct {
	ent    *ent.Client
	bus    *events.Bus
	cipher *secrets.Cipher
	now    func() time.Time
}

// New builds the use case. now is the Instance clock TOTP codes are checked against.
func New(client *ent.Client, bus *events.Bus, cipher *secrets.Cipher, now func() time.Time) *UseCase {
	return &UseCase{ent: client, bus: bus, cipher: cipher, now: now}
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
func (m *UseCase) Status(ctx context.Context, userID int64) (Status, error) {
	u, err := m.ent.User.Get(ctx, userID)
	if err != nil {
		return Status{}, err
	}
	if !users.HasSecondFactor(u) {
		return Status{Pending: u.SecondFactorSecretEncrypted != ""}, nil
	}
	n, err := recoverycodes.Unused(ctx, m.ent, userID)
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

// StartEnrollment creates a pending secret for the User, replacing an earlier
// pending one. ErrAlreadyActive when a Second factor is active: replacing it takes
// disabling it first (password and a current code).
func (m *UseCase) StartEnrollment(ctx context.Context, userID int64) (*Enrollment, error) {
	var enrollment *Enrollment
	err := m.bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		u, err := tx.User.Get(ctx, userID)
		if err != nil {
			return err
		}
		if users.HasSecondFactor(u) {
			return ErrAlreadyActive
		}
		key, err := totp.New(users.Issuer, u.Email)
		if err != nil {
			return err
		}
		sealed, err := m.cipher.Encrypt([]byte(key.Secret))
		if err != nil {
			return err
		}
		u, err = users.MoveSecondFactorIn(ctx, tx, u, users.SecondFactorChange{Trigger: users.FactorStart, SealedSecret: sealed, At: m.now()})
		if err != nil {
			return err
		}
		enrollment = &Enrollment{ID: *u.SecondFactorGeneration, Secret: key.Secret, URI: key.URI, QRCode: key.QRCode}
		return nil
	})
	return enrollment, err
}

// ConfirmInput binds the code to the enrollment whose secret the caller received.
type ConfirmInput struct {
	EnrollmentID int64
	Code         string
}

// ConfirmEnrollment activates the pending secret when code is a current code for
// it. It issues the Recovery codes (returned once), bumps the session epoch and
// records `user.second_factor_enroll`, all in one transaction, and returns the
// stored User for reissuing the acting session with the codes. ErrNoPending without a pending
// enrollment, ErrInvalidCode on a wrong code.
func (m *UseCase) ConfirmEnrollment(ctx context.Context, userID int64, in ConfirmInput) (*CodesIssued, error) {
	var (
		saved *ent.User
		codes []string
	)
	err := m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		u, err := tx.User.Get(ctx, userID)
		if err != nil {
			return err
		}
		if users.HasSecondFactor(u) {
			return ErrAlreadyActive
		}
		if u.SecondFactorSecretEncrypted == "" {
			return ErrNoPending
		}
		if u.SecondFactorGeneration == nil || *u.SecondFactorGeneration != in.EnrollmentID {
			return lifecycle.ErrInvalidTransition
		}
		step, err := m.totpStep(u, in.Code)
		if err != nil {
			return err
		}
		u, err = users.MoveSecondFactorIn(ctx, tx, u, users.SecondFactorChange{Trigger: users.FactorConfirm, Step: step, At: m.now()})
		if err != nil {
			return err
		}
		if codes, err = recoverycodes.ReplaceIn(ctx, tx, userID); err != nil {
			return err
		}
		saved = u
		return users.RecordUserAction(ctx, tx, pub, u, events.ActionUserSecondFactorEnroll, nil)
	})
	if err != nil {
		return nil, err
	}
	return &CodesIssued{User: saved, RecoveryCodes: codes}, nil
}

// RegenerateRecoveryCodes replaces the User's Recovery codes with a fresh set
// (returned once; the previous set stops working), bumps the session epoch and
// records `user.recovery_codes_regenerate`. ErrNotActive without a Second factor.
func (m *UseCase) RegenerateRecoveryCodes(ctx context.Context, userID int64) (*CodesIssued, error) {
	var (
		saved *ent.User
		codes []string
	)
	err := m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		u, err := tx.User.Get(ctx, userID)
		if err != nil {
			return err
		}
		if !users.HasSecondFactor(u) {
			return ErrNotActive
		}
		if u, err = users.MoveSecondFactorIn(ctx, tx, u, users.SecondFactorChange{Trigger: users.FactorRegenerate, At: m.now()}); err != nil {
			return err
		}
		if codes, err = recoverycodes.ReplaceIn(ctx, tx, userID); err != nil {
			return err
		}
		saved = u
		return users.RecordUserAction(ctx, tx, pub, u, events.ActionUserRecoveryCodesRegenerate, nil)
	})
	if err != nil {
		return nil, err
	}
	return &CodesIssued{User: saved, RecoveryCodes: codes}, nil
}

// Disable removes the User's Second factor when code is a current TOTP code or an
// unused Recovery code (the caller has checked the password). It clears the factor
// and its Recovery codes, bumps the session epoch and records
// `user.second_factor_disable`. ErrNotActive without a Second factor,
// ErrInvalidCode on a wrong code.
func (m *UseCase) Disable(ctx context.Context, userID int64, code string) (*ent.User, error) {
	var saved *ent.User
	err := m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		u, err := tx.User.Get(ctx, userID)
		if err != nil {
			return err
		}
		if !users.HasSecondFactor(u) {
			return ErrNotActive
		}
		_, verified, err := m.verify(ctx, tx, pub, u, code)
		if err != nil {
			return err
		}
		if saved, err = remove(ctx, tx, verified, users.FactorDisable, m.now()); err != nil {
			return err
		}
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
func (m *UseCase) Verify(ctx context.Context, pending login.Pending[*ent.User], code string) (Method, error) {
	binding := pending.Binding
	var method Method
	err := m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		u, err := tx.User.Query().Where(user.ID(pending.Principal.ID)).ForUpdate().Only(ctx)
		if err != nil {
			return err
		}
		if !users.HasSecondFactor(u) {
			return ErrNotActive
		}
		current, err := challengeBinding(ctx, tx, u)
		if err != nil {
			return err
		}
		if subtle.ConstantTimeCompare([]byte(current), []byte(binding)) != 1 {
			return ErrBindingMoved
		}
		method, _, err = m.verify(ctx, tx, pub, u, code)
		return err
	})
	return method, err
}

// DeletionCode is the users.DeleteConfirm of User deletion (ADR 0050): with a Second factor
// it spends code in the deletion's transaction (ErrDeletionCodeInvalid on a wrong one), so the
// code is spent only if the deletion commits; without one it confirms nothing.
func (m *UseCase) DeletionCode(code string) users.DeleteConfirm {
	return func(ctx context.Context, tx *ent.Client, pub events.Publisher, locked *ent.User) error {
		if !users.HasSecondFactor(locked) {
			return nil
		}
		_, _, err := m.verify(ctx, tx, pub, locked, code)
		if errors.Is(err, ErrInvalidCode) {
			return ErrDeletionCodeInvalid
		}
		return err
	}
}

// ChallengeBinding is the value a login challenge (ADR 0020) is keyed to: it
// changes on every successful Verify (a TOTP success moves the last accepted time
// step, a Recovery code success lowers the count of unused codes), on a password
// change and on every session epoch bump (enroll, regenerate, disable, reset).
// Keying the challenge's signature to it makes the challenge single-use without a
// store of spent challenges, so it holds across instances; Verify re-checks it
// under a lock, so two racing second steps cannot both use it. ErrNotActive
// without a Second factor.
func (m *UseCase) ChallengeBinding(ctx context.Context, u *ent.User) (string, error) {
	if !users.HasSecondFactor(u) {
		return "", ErrNotActive
	}
	return challengeBinding(ctx, m.ent, u)
}

func challengeBinding(ctx context.Context, client *ent.Client, u *ent.User) (string, error) {
	unused, err := recoverycodes.Unused(ctx, client, u.ID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d|%d|%d|%s", u.SessionEpoch, u.SecondFactorLastStep, unused, u.PasswordHash), nil
}

// Reset clears the User's Second factor, Recovery codes and Passkeys and bumps the session
// epoch, ending every session of theirs: an Owner or Admin resetting a member of the
// Workspace behind s. It records `user.second_factor_reset` by actor in that Workspace
// only. ErrNothingToReset without an active or pending factor or a Passkey (nothing changes).
func (m *UseCase) Reset(ctx context.Context, s *ent.Scoped, userID int64, actor events.Actor) error {
	return m.reset(ctx, userID, func(pub events.Publisher, _ *ent.Client, u *ent.User) error {
		return users.RecordActionOnUserIn(ctx, pub, actor, u, events.ActionUserSecondFactorReset, nil, s.WorkspaceID())
	})
}

// ResetByStaff is Reset from the staff member command: the platform Staff member `by`
// (shown to customers as "Sphericon staff") is recorded in every Workspace the User
// holds a Membership in.
func (m *UseCase) ResetByStaff(ctx context.Context, userID int64, by string) error {
	actor := events.Actor{Kind: events.ActorStaff, ID: by}
	return m.reset(ctx, userID, func(pub events.Publisher, tx *ent.Client, u *ent.User) error {
		return users.RecordActionOnUser(ctx, tx, pub, actor, u, events.ActionUserSecondFactorReset, nil)
	})
}

func (m *UseCase) reset(ctx context.Context, userID int64, record func(events.Publisher, *ent.Client, *ent.User) error) error {
	return m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		u, err := tx.User.Get(ctx, userID)
		if err != nil {
			return err
		}
		held, err := passkeys.HeldIn(ctx, tx, userID)
		if err != nil {
			return err
		}
		if u.SecondFactorSecretEncrypted == "" && !held {
			return ErrNothingToReset
		}
		if u, err = remove(ctx, tx, u, users.FactorReset, m.now()); err != nil {
			return err
		}
		if err := passkeys.DeleteAllIn(ctx, tx, userID); err != nil {
			return err
		}
		return record(pub, tx, u)
	})
}

// verify fences the User before touching Recovery codes, keeping the same lock order as
// enrollment, regeneration, removal and the login challenge check.
func (m *UseCase) verify(ctx context.Context, tx *ent.Client, pub events.Publisher, u *ent.User, code string) (Method, *ent.User, error) {
	step := int64(0)
	if totp.IsCode(code) {
		var err error
		step, err = m.totpStep(u, code)
		if err != nil {
			return "", nil, err
		}
	}
	saved, err := users.MoveSecondFactorIn(ctx, tx, u, users.SecondFactorChange{Trigger: users.FactorVerify, Step: step, At: m.now()})
	if err != nil {
		return "", nil, err
	}
	if step > 0 {
		return MethodTOTP, saved, nil
	}
	spent, err := recoverycodes.SpendIn(ctx, tx, u.ID, code, m.now())
	if err != nil {
		return "", nil, err
	}
	if !spent {
		return "", nil, ErrInvalidCode
	}
	err = users.RecordUserAction(ctx, tx, pub, saved, events.ActionUserRecoveryCodeUse, nil)
	return MethodRecoveryCode, saved, err
}

// totpStep checks the code against this exact secret and replay step. The owner consumes
// both in its single conditional lifecycle write; no separate acceptance can outlive a reset.
func (m *UseCase) totpStep(u *ent.User, code string) (int64, error) {
	raw, err := m.cipher.Decrypt(u.SecondFactorSecretEncrypted)
	if err != nil {
		return 0, err
	}
	step, ok, err := totp.Match(string(raw), code, m.now(), u.SecondFactorLastStep)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, ErrInvalidCode
	}
	return step, nil
}

// remove clears the factor before its Recovery codes, and bumps the epoch in the same transaction.
func remove(ctx context.Context, tx *ent.Client, before *ent.User, trigger users.SecondFactorTrigger, at time.Time) (*ent.User, error) {
	saved, err := users.MoveSecondFactorIn(ctx, tx, before, users.SecondFactorChange{Trigger: trigger, At: at})
	if err != nil {
		return nil, err
	}
	if err := recoverycodes.DeleteIn(ctx, tx, before.ID); err != nil {
		return nil, err
	}
	return saved, nil
}
