package users

import (
	"context"
	"time"

	"github.com/qmuntal/stateless"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
)

// Issuer names the Instance to authenticator apps and Passkeys.
const Issuer = "sphericon"

// SecondFactorState projects the secret and confirmation moment, without a second status column.
type SecondFactorState string

const (
	FactorNone    SecondFactorState = "none"
	FactorPending SecondFactorState = "pending"
	FactorActive  SecondFactorState = "active"
)

// SecondFactorTrigger is one operation on the User's factor.
type SecondFactorTrigger string

const (
	FactorStart      SecondFactorTrigger = "start"
	FactorConfirm    SecondFactorTrigger = "confirm"
	FactorVerify     SecondFactorTrigger = "verify"
	FactorRegenerate SecondFactorTrigger = "regenerate"
	FactorDisable    SecondFactorTrigger = "disable"
	FactorReset      SecondFactorTrigger = "reset"
)

// SecondFactorChange carries the new secret on Start, or the matched TOTP step on Confirm
// and Verify. A zero step on Verify fences recovery-code consumption in the same transaction.
type SecondFactorChange struct {
	Trigger      SecondFactorTrigger
	SealedSecret string
	Step         int64
	At           time.Time
}

func secondFactorState(u *ent.User) SecondFactorState {
	if u.SecondFactorSecretEncrypted == "" {
		return FactorNone
	}
	if u.SecondFactorConfirmedAt == nil {
		return FactorPending
	}
	return FactorActive
}

// SecondFactorMachine declares the entire lifecycle. Fire change.Trigger once in the
// transaction of write. Internal transitions call write explicitly: stateless does not call
// its storage mutator for them. Reset from none also fences removal of the User's Passkeys;
// the use case checks that there is something to reset.
func SecondFactorMachine(before *ent.User, change SecondFactorChange, write func(context.Context, SecondFactorState) (bool, error)) *stateless.StateMachine {
	current := secondFactorState(before)
	persist := func(ctx context.Context, to SecondFactorState) (bool, error) {
		swapped, err := write(ctx, to)
		if err == nil && swapped {
			current = to
		}
		return swapped, err
	}
	machine := lifecycle.New(lifecycle.Storage[SecondFactorState]{
		Read:  func(context.Context) (SecondFactorState, error) { return current, nil },
		Write: func(ctx context.Context, _, to SecondFactorState) (bool, error) { return persist(ctx, to) },
	})
	internal := func(ctx context.Context, _ ...any) error {
		swapped, err := persist(ctx, current)
		if err != nil {
			return err
		}
		if !swapped {
			return lifecycle.ErrInvalidTransition
		}
		return nil
	}
	hasSecret := func(context.Context, ...any) bool { return change.SealedSecret != "" }
	freshStep := func(context.Context, ...any) bool { return change.Step > before.SecondFactorLastStep }
	verifyStep := func(context.Context, ...any) bool {
		return change.Step == 0 || change.Step > before.SecondFactorLastStep
	}
	machine.Configure(FactorNone).
		Permit(FactorStart, FactorPending, hasSecret).
		InternalTransition(FactorReset, internal)
	machine.Configure(FactorPending).
		InternalTransition(FactorStart, internal, hasSecret).
		Permit(FactorConfirm, FactorActive, freshStep).
		Permit(FactorReset, FactorNone)
	machine.Configure(FactorActive).
		InternalTransition(FactorVerify, internal, verifyStep).
		InternalTransition(FactorRegenerate, internal).
		Permit(FactorDisable, FactorNone).
		Permit(FactorReset, FactorNone)
	return machine
}

// MoveSecondFactorIn atomically consumes the snapshot that was checked, including its secret,
// enrollment identity, epoch and accepted step. A stale operation never acts on a newer factor.
// The caller writes Recovery codes and audit/outbox entries on tx after this move.
func MoveSecondFactorIn(ctx context.Context, tx *ent.Client, before *ent.User, change SecondFactorChange) (*ent.User, error) {
	var saved *ent.User
	machine := SecondFactorMachine(before, change, func(ctx context.Context, _ SecondFactorState) (bool, error) {
		update := tx.User.UpdateOneID(before.ID).Where(secondFactorSnapshot(before)...)
		switch change.Trigger {
		case FactorStart:
			generation := int64(1)
			if before.SecondFactorGeneration != nil {
				generation += *before.SecondFactorGeneration
			}
			update.SetSecondFactorGeneration(generation).SetSecondFactorSecretEncrypted(change.SealedSecret).
				ClearSecondFactorConfirmedAt().SetSecondFactorLastStep(0)
		case FactorConfirm:
			update.SetSecondFactorConfirmedAt(change.At).SetSecondFactorLastStep(change.Step).AddSessionEpoch(1)
		case FactorVerify:
			if change.Step > 0 {
				update.SetSecondFactorLastStep(change.Step)
			} else {
				update.SetSecondFactorLastStep(before.SecondFactorLastStep)
			}
		case FactorRegenerate:
			update.AddSessionEpoch(1)
		case FactorDisable, FactorReset:
			update.ClearSecondFactorSecretEncrypted().ClearSecondFactorConfirmedAt().SetSecondFactorLastStep(0).AddSessionEpoch(1)
		}
		var err error
		saved, err = update.Save(ctx)
		if ent.IsNotFound(err) {
			return false, nil
		}
		return err == nil, err
	})
	if err := machine.FireCtx(ctx, change.Trigger); err != nil {
		return nil, err
	}
	return saved, nil
}

func secondFactorSnapshot(before *ent.User) []predicate.User {
	predicates := []predicate.User{user.SessionEpochEQ(before.SessionEpoch), user.SecondFactorLastStepEQ(before.SecondFactorLastStep)}
	if before.SecondFactorSecretEncrypted == "" {
		predicates = append(predicates, user.Or(user.SecondFactorSecretEncryptedIsNil(), user.SecondFactorSecretEncryptedEQ("")))
	} else {
		predicates = append(predicates, user.SecondFactorSecretEncryptedEQ(before.SecondFactorSecretEncrypted))
	}
	if before.SecondFactorConfirmedAt == nil {
		predicates = append(predicates, user.SecondFactorConfirmedAtIsNil())
	} else {
		predicates = append(predicates, user.SecondFactorConfirmedAtEQ(*before.SecondFactorConfirmedAt))
	}
	if before.SecondFactorGeneration == nil {
		predicates = append(predicates, user.SecondFactorGenerationIsNil())
	} else {
		predicates = append(predicates, user.SecondFactorGenerationEQ(*before.SecondFactorGeneration))
	}
	// Passwordless Users store no password as NULL or an empty optional string.
	if before.PasswordHash == "" {
		predicates = append(predicates, user.Or(user.PasswordHashIsNil(), user.PasswordHashEQ("")))
	} else {
		predicates = append(predicates, user.PasswordHashEQ(before.PasswordHash))
	}
	return predicates
}

// BumpSessionEpochIn ends the User's sessions (also used by Passkey changes).
func BumpSessionEpochIn(ctx context.Context, tx *ent.Client, userID int64) (*ent.User, error) {
	return tx.User.UpdateOneID(userID).AddSessionEpoch(1).Save(ctx)
}
