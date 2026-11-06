// Package secondfactorenrolment is the use case of a User's TOTP Second factor and Recovery
// codes (ADR 0020, ADR 0102): enrolling, verifying a code, regenerating the codes, disabling and
// resetting, each an operation across the User (internal/users, which owns the factor's columns
// and the session epoch), the Recovery codes (internal/recoverycodes) and, for a reset, the
// Passkeys (internal/passkeys). It checks the code or the state first and writes each row through
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
// docs/architecture.md (Workspace scoping).
package secondfactorenrolment

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/otpcode"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
	"github.com/getsphericon/sphericon/internal/passkeys"
	"github.com/getsphericon/sphericon/internal/recoverycodes"
	"github.com/getsphericon/sphericon/internal/users"
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
	// ErrNothingToReset: the member has neither a Second factor nor a Passkey, so a reset has
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
	ErrBindingMoved = errors.New("secondfactor: challenge binding moved")
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

// New builds the use case. now is the clock TOTP codes are checked against (nil
// means time.Now); tests inject one.
func New(client *ent.Client, bus *events.Bus, cipher *secrets.Cipher, now func() time.Time) *UseCase {
	if now == nil {
		now = time.Now
	}
	return &UseCase{ent: client, bus: bus, cipher: cipher, now: now}
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
	Secret string // base32 key
	URI    string // otpauth:// URI
	QRCode string // PNG data URI of the URI
}

// StartEnrollment creates a pending secret for the User, replacing an earlier
// pending one. ErrAlreadyActive when a Second factor is active: replacing it takes
// disabling it first (password and a current code).
func (m *UseCase) StartEnrollment(ctx context.Context, userID int64) (*Enrollment, error) {
	u, err := m.ent.User.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	if users.HasSecondFactor(u) {
		return nil, ErrAlreadyActive
	}
	key, err := otpcode.New(users.Issuer, u.Email)
	if err != nil {
		return nil, err
	}
	sealed, err := m.cipher.Encrypt([]byte(key.Secret))
	if err != nil {
		return nil, err
	}
	if err := users.SetPendingSecondFactor(ctx, m.ent, userID, sealed); err != nil {
		return nil, err
	}
	return &Enrollment{Secret: key.Secret, URI: key.URI, QRCode: key.QRCode}, nil
}

// ConfirmEnrollment activates the pending secret when code is a current code for
// it. It issues the Recovery codes (returned once), bumps the session epoch and
// records `user.second_factor_enroll`, all in one transaction, and returns the
// stored User for reissuing the acting session. ErrNoPending without a pending
// enrollment, ErrInvalidCode on a wrong code.
func (m *UseCase) ConfirmEnrollment(ctx context.Context, userID int64, code string) (*ent.User, []string, error) {
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
		if err := m.useTOTP(ctx, tx, u, code); err != nil {
			return err
		}
		u, err = users.ConfirmSecondFactorIn(ctx, tx, userID, m.now())
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
		return nil, nil, err
	}
	return saved, codes, nil
}

// RegenerateRecoveryCodes replaces the User's Recovery codes with a fresh set
// (returned once; the previous set stops working), bumps the session epoch and
// records `user.recovery_codes_regenerate`. ErrNotActive without a Second factor.
func (m *UseCase) RegenerateRecoveryCodes(ctx context.Context, userID int64) (*ent.User, []string, error) {
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
		if codes, err = recoverycodes.ReplaceIn(ctx, tx, userID); err != nil {
			return err
		}
		if u, err = users.BumpSessionEpochIn(ctx, tx, userID); err != nil {
			return err
		}
		saved = u
		return users.RecordUserAction(ctx, tx, pub, u, events.ActionUserRecoveryCodesRegenerate, nil)
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
		if _, err := m.verify(ctx, tx, pub, u, code); err != nil {
			return err
		}
		if saved, err = remove(ctx, tx, userID); err != nil {
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
// binding is the ChallengeBinding the step's challenge was minted under. It is
// re-checked inside the transaction with the User row locked, so of two second
// steps racing on one challenge (say one with a TOTP code, one with a Recovery
// code) the later sees the binding the earlier moved and fails: a challenge starts
// at most one session. ErrNotActive without a Second factor, ErrBindingMoved when
// the binding moved, ErrInvalidCode otherwise.
func (m *UseCase) Verify(ctx context.Context, userID int64, binding, code string) (Method, error) {
	var method Method
	err := m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		u, err := tx.User.Query().Where(user.ID(userID)).ForUpdate().Only(ctx)
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
		method, err = m.verify(ctx, tx, pub, u, code)
		return err
	})
	return method, err
}

// VerifyIn checks a code for the User u inside the caller's transaction tx (with pub its
// publisher), for an action that needs the Second factor's proof and its own atomicity: the
// code is spent (a TOTP step used, a Recovery code consumed) only if the caller's
// transaction commits. The caller holds u's row locked. ErrNotActive without a Second
// factor, ErrInvalidCode on a wrong code.
func (m *UseCase) VerifyIn(ctx context.Context, tx *ent.Client, pub events.Publisher, u *ent.User, code string) (Method, error) {
	if !users.HasSecondFactor(u) {
		return "", ErrNotActive
	}
	return m.verify(ctx, tx, pub, u, code)
}

// DeletionCode is the users.DeleteConfirm of User deletion (ADR 0050): with a Second factor
// it spends code in the deletion's transaction (ErrDeletionCodeInvalid on a wrong one), so the
// code is spent only if the deletion commits; without one it confirms nothing.
func (m *UseCase) DeletionCode(code string) users.DeleteConfirm {
	return func(ctx context.Context, tx *ent.Client, pub events.Publisher, locked *ent.User) error {
		if !users.HasSecondFactor(locked) {
			return nil
		}
		_, err := m.VerifyIn(ctx, tx, pub, locked, code)
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
// epoch, ending every session of theirs: an Owner or Admin resetting a member of
// workspaceID. It records `user.second_factor_reset` by actor in that Workspace
// only. ErrNothingToReset without a Second factor or a Passkey (nothing changes).
func (m *UseCase) Reset(ctx context.Context, userID int64, actor events.Actor, workspaceID int64) error {
	return m.reset(ctx, userID, func(pub events.Publisher, _ *ent.Client, u *ent.User) error {
		return users.RecordActionOnUserIn(ctx, pub, actor, u, events.ActionUserSecondFactorReset, nil, workspaceID)
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
		if !users.HasSecondFactor(u) && !held {
			return ErrNothingToReset
		}
		if err := passkeys.DeleteAllIn(ctx, tx, userID); err != nil {
			return err
		}
		if u, err = remove(ctx, tx, userID); err != nil {
			return err
		}
		return record(pub, tx, u)
	})
}

func (m *UseCase) verify(ctx context.Context, tx *ent.Client, pub events.Publisher, u *ent.User, code string) (Method, error) {
	if otpcode.IsCode(code) {
		if err := m.useTOTP(ctx, tx, u, code); err != nil {
			return "", err
		}
		return MethodTOTP, nil
	}
	spent, err := recoverycodes.SpendIn(ctx, tx, u.ID, code, m.now())
	if err != nil {
		return "", err
	}
	if !spent {
		return "", ErrInvalidCode
	}
	return MethodRecoveryCode, users.RecordUserAction(ctx, tx, pub, u, events.ActionUserRecoveryCodeUse, nil)
}

// useTOTP accepts code when it is the User's code for a time step within the skew
// of now and later than the last step accepted, and stores that step, so the code
// cannot be replayed. The conditional write makes two concurrent uses of one code
// accept only one.
func (m *UseCase) useTOTP(ctx context.Context, tx *ent.Client, u *ent.User, code string) error {
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
	accepted, err := users.AcceptSecondFactorStepIn(ctx, tx, u.ID, step)
	if err != nil {
		return err
	}
	if !accepted {
		return ErrInvalidCode
	}
	return nil
}

// remove clears the Second factor and the Recovery codes and bumps the epoch.
func remove(ctx context.Context, tx *ent.Client, userID int64) (*ent.User, error) {
	if err := recoverycodes.DeleteIn(ctx, tx, userID); err != nil {
		return nil, err
	}
	return users.ClearSecondFactorIn(ctx, tx, userID)
}
