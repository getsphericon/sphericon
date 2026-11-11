package secondfactors

import (
	"context"
	"errors"
	"time"

	"github.com/qmuntal/stateless"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/secondfactor"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
)

// State projects the secret and confirmation moment, without a status column of its own.
type State string

const (
	FactorNone    State = "none"
	FactorPending State = "pending"
	FactorActive  State = "active"
)

// Trigger is one operation on the User's factor.
type Trigger string

const (
	FactorStart      Trigger = "start"
	FactorConfirm    Trigger = "confirm"
	FactorVerify     Trigger = "verify"
	FactorRegenerate Trigger = "regenerate"
	FactorDisable    Trigger = "disable"
	FactorReset      Trigger = "reset"
)

// Change carries the new secret on Start, or the matched TOTP step on Confirm and Verify. A
// zero step on Verify fences recovery-code consumption in the same transaction.
type Change struct {
	Trigger      Trigger
	SealedSecret string
	Step         int64
	At           time.Time
}

// stateOf is the state of the factor row f; nil is a User who never enrolled.
func stateOf(f *ent.SecondFactor) State {
	if f == nil || f.SecretEncrypted == "" {
		return FactorNone
	}
	if f.ConfirmedAt == nil {
		return FactorPending
	}
	return FactorActive
}

// active reports whether f is a confirmed Second factor. A pending enrollment is not one.
func active(f *ent.SecondFactor) bool { return stateOf(f) == FactorActive }

// lastStep is the last accepted TOTP step of f, 0 without a factor row.
func lastStep(f *ent.SecondFactor) int64 {
	if f == nil {
		return 0
	}
	return f.LastStep
}

// Machine declares the entire lifecycle of the factor row before (nil for a User who never
// enrolled). Fire change.Trigger once in the transaction of storage. storage.Write is the
// compare-and-swap of the factor row (ADR 0109); its Read is not called, since the machine reads
// the state of before. Internal transitions call Write explicitly: stateless does not call its
// storage mutator for them. Reset from none also fences removal of the User's Passkeys; the reset
// use case checks that there is something to reset.
func Machine(before *ent.SecondFactor, change Change, storage lifecycle.Storage[State]) *stateless.StateMachine {
	current := stateOf(before)
	persist := func(ctx context.Context, to State) (bool, error) {
		swapped, err := storage.Write(ctx, current, to)
		if err == nil && swapped {
			current = to
		}
		return swapped, err
	}
	machine := lifecycle.New(lifecycle.Storage[State]{
		Read:  func(context.Context) (State, error) { return current, nil },
		Write: func(ctx context.Context, _, to State) (bool, error) { return persist(ctx, to) },
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
	freshStep := func(context.Context, ...any) bool { return change.Step > lastStep(before) }
	verifyStep := func(context.Context, ...any) bool {
		return change.Step == 0 || change.Step > lastStep(before)
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

// endsSessions is whether a change ends every session of the User: confirming, regenerating the
// Recovery codes, disabling and resetting bump the session epoch, opening a QR code and a second
// step do not.
func endsSessions(t Trigger) bool {
	switch t {
	case FactorConfirm, FactorRegenerate, FactorDisable, FactorReset:
		return true
	case FactorStart, FactorVerify:
		return false
	}
	return false
}

// Snapshot is what a change checked: the User (its session epoch and password) and its factor
// row (nil when it never enrolled).
type Snapshot struct {
	User   *ent.User
	Factor *ent.SecondFactor
}

// Moved is the outcome of a change: the stored User (its epoch moved when the change ends
// sessions) and factor row.
type Moved struct {
	User   *ent.User
	Factor *ent.SecondFactor
}

// errResetUnchecked refuses a reset in move: the reset is written only by
// secondfactorsunchecked.ResetIn, behind the Second factor reset's check.
var errResetUnchecked = errors.New("secondfactors: a reset is written by secondfactorsunchecked.ResetIn")

// move atomically consumes the snapshot that was checked: the User's session epoch and password
// and the factor's secret, enrollment identity, confirmation and accepted step. A stale operation
// never acts on a newer factor or on a User whose password or sessions moved meanwhile. The User
// row is written before the factor row, the order every change and the second step lock them in.
// The caller writes Recovery codes and audit/outbox entries on tx after this move.
func move(ctx context.Context, tx *ent.Client, before Snapshot, change Change) (Moved, error) {
	out := Moved(before)
	machine := Machine(before.Factor, change, lifecycle.Storage[State]{Write: func(ctx context.Context, _, _ State) (bool, error) {
		if endsSessions(change.Trigger) {
			saved, bumped, err := users.BumpSessionEpochFromIn(ctx, tx, before.User)
			if err != nil || !bumped {
				return false, err
			}
			out.User = saved
		}
		if before.Factor == nil {
			return writeFirst(ctx, tx, before.User.ID, change, &out)
		}
		update := tx.SecondFactor.UpdateOneID(before.Factor.ID).Where(Unchanged(before.Factor)...)
		switch change.Trigger {
		case FactorStart:
			update.SetGeneration(before.Factor.Generation + 1).SetSecretEncrypted(change.SealedSecret).
				ClearConfirmedAt().SetLastStep(0)
		case FactorConfirm:
			update.SetConfirmedAt(change.At).SetLastStep(change.Step)
		case FactorVerify:
			if change.Step > 0 {
				update.SetLastStep(change.Step)
			} else {
				update.SetLastStep(before.Factor.LastStep)
			}
		case FactorRegenerate:
			update.SetLastStep(before.Factor.LastStep)
		case FactorDisable:
			update.ClearSecretEncrypted().ClearConfirmedAt().SetLastStep(0)
		case FactorReset:
			// A reset clears the factor without its code: it is the unchecked
			// secondfactorsunchecked.ResetIn's write (ADR 0110), never this module's.
			return false, errResetUnchecked
		}
		saved, err := update.Save(ctx)
		if ent.IsNotFound(err) {
			return false, nil
		}
		out.Factor = saved
		return err == nil, err
	}})
	if err := machine.FireCtx(ctx, change.Trigger); err != nil {
		return Moved{}, err
	}
	return out, nil
}

// writeFirst is the change of a User who never enrolled: Start creates the factor row with the
// first enrollment identity (false when a concurrent first Start got there first), and a Reset
// (of a User who holds only Passkeys) has no factor row to write.
func writeFirst(ctx context.Context, tx *ent.Client, userID int64, change Change, out *Moved) (bool, error) {
	if change.Trigger != FactorStart {
		return true, nil
	}
	id, err := tx.SecondFactor.Create().
		SetUserID(userID).
		SetGeneration(1).
		SetSecretEncrypted(change.SealedSecret).
		OnConflictColumns(secondfactor.FieldUserID).
		DoNothing().
		ID(ctx)
	created, err := db.Inserted(err)
	if err != nil || !created {
		return false, err
	}
	out.Factor, err = tx.SecondFactor.Get(ctx, id)
	return err == nil, err
}

// Unchanged is the condition the factor row is still before: its enrollment identity, secret,
// confirmation and accepted step. Every change of the row is a compare-and-swap on it, so a stale
// operation never acts on a newer factor.
func Unchanged(before *ent.SecondFactor) []predicate.SecondFactor {
	predicates := []predicate.SecondFactor{
		secondfactor.GenerationEQ(before.Generation),
		secondfactor.LastStepEQ(before.LastStep),
	}
	if before.SecretEncrypted == "" {
		predicates = append(predicates, secondfactor.Or(secondfactor.SecretEncryptedIsNil(), secondfactor.SecretEncryptedEQ("")))
	} else {
		predicates = append(predicates, secondfactor.SecretEncryptedEQ(before.SecretEncrypted))
	}
	if before.ConfirmedAt == nil {
		predicates = append(predicates, secondfactor.ConfirmedAtIsNil())
	} else {
		predicates = append(predicates, secondfactor.ConfirmedAtEQ(*before.ConfirmedAt))
	}
	return predicates
}
