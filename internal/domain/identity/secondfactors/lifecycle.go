package secondfactors

import (
	"context"
	"time"

	"github.com/qmuntal/stateless"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/secondfactor"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
)

// state projects the secret and confirmation moment, without a status column of its own.
type state string

const (
	factorNone    state = "none"
	factorPending state = "pending"
	factorActive  state = "active"
)

// trigger is one operation on the User's factor.
type trigger string

const (
	factorStart      trigger = "start"
	factorConfirm    trigger = "confirm"
	factorVerify     trigger = "verify"
	factorRegenerate trigger = "regenerate"
	factorDisable    trigger = "disable"
	factorReset      trigger = "reset"
)

// change carries the new secret on Start, or the matched TOTP step on Confirm and Verify. A
// zero step on Verify fences recovery-code consumption in the same transaction.
type change struct {
	trigger      trigger
	sealedSecret string
	step         int64
	at           time.Time
}

// stateOf is the state of the factor row f; nil is a User who never enrolled.
func stateOf(f *ent.SecondFactor) state {
	if f == nil || f.SecretEncrypted == "" {
		return factorNone
	}
	if f.ConfirmedAt == nil {
		return factorPending
	}
	return factorActive
}

// active reports whether f is a confirmed Second factor. A pending enrollment is not one.
func active(f *ent.SecondFactor) bool { return stateOf(f) == factorActive }

// lastStep is the last accepted TOTP step of f, 0 without a factor row.
func lastStep(f *ent.SecondFactor) int64 {
	if f == nil {
		return 0
	}
	return f.LastStep
}

// newMachine declares the entire lifecycle of the factor row before (nil for a User who never
// enrolled). Fire c.trigger once in the transaction of storage. storage.Write is the
// compare-and-swap of the factor row (ADR 0109); its Read is not called, since the machine reads
// the state of before. Internal transitions call Write explicitly: stateless does not call its
// storage mutator for them. A reset from none (a User who holds only Passkeys) still bumps the
// session epoch, which fences the removal of the Passkeys; resetIn checks that there is something
// to reset before it fires.
func newMachine(before *ent.SecondFactor, c change, storage lifecycle.Storage[state]) *stateless.StateMachine {
	current := stateOf(before)
	persist := func(ctx context.Context, to state) (bool, error) {
		swapped, err := storage.Write(ctx, current, to)
		if err == nil && swapped {
			current = to
		}
		return swapped, err
	}
	machine := lifecycle.New(lifecycle.Storage[state]{
		Read:  func(context.Context) (state, error) { return current, nil },
		Write: func(ctx context.Context, _, to state) (bool, error) { return persist(ctx, to) },
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
	hasSecret := func(context.Context, ...any) bool { return c.sealedSecret != "" }
	freshStep := func(context.Context, ...any) bool { return c.step > lastStep(before) }
	verifyStep := func(context.Context, ...any) bool {
		return c.step == 0 || c.step > lastStep(before)
	}
	machine.Configure(factorNone).
		Permit(factorStart, factorPending, hasSecret).
		InternalTransition(factorReset, internal)
	machine.Configure(factorPending).
		InternalTransition(factorStart, internal, hasSecret).
		Permit(factorConfirm, factorActive, freshStep).
		Permit(factorReset, factorNone)
	machine.Configure(factorActive).
		InternalTransition(factorVerify, internal, verifyStep).
		InternalTransition(factorRegenerate, internal).
		Permit(factorDisable, factorNone).
		Permit(factorReset, factorNone)
	return machine
}

// endsSessions is whether a change ends every session of the User: confirming, regenerating the
// Recovery codes, disabling and resetting bump the session epoch, opening a QR code and a second
// step do not.
func endsSessions(t trigger) bool {
	switch t {
	case factorConfirm, factorRegenerate, factorDisable, factorReset:
		return true
	case factorStart, factorVerify:
		return false
	}
	return false
}

// snapshot is what a change checked: the User (its session epoch and password) and its factor
// row (nil when it never enrolled).
type snapshot struct {
	User   *ent.User
	Factor *ent.SecondFactor
}

// outcome is what a change leaves: the stored User (its epoch moved when the change ends
// sessions) and factor row.
type outcome struct {
	User   *ent.User
	Factor *ent.SecondFactor
}

// move atomically consumes the snapshot that was checked: the User's session epoch and password
// and the factor's secret, enrollment identity, confirmation and accepted step. A stale operation
// never acts on a newer factor or on a User whose password or sessions moved meanwhile. The User
// row is written before the factor row, the order every change and the second step lock them in.
// The caller writes Recovery codes and audit/outbox entries on tx after this move.
func move(ctx context.Context, tx *ent.Client, before snapshot, c change) (outcome, error) {
	out := outcome(before)
	machine := newMachine(before.Factor, c, lifecycle.Storage[state]{Write: func(ctx context.Context, _, _ state) (bool, error) {
		if endsSessions(c.trigger) {
			saved, bumped, err := users.BumpSessionEpochFromIn(ctx, tx, before.User)
			if err != nil || !bumped {
				return false, err
			}
			out.User = saved
		}
		if before.Factor == nil {
			return writeFirst(ctx, tx, before.User.ID, c, &out)
		}
		update := tx.SecondFactor.UpdateOneID(before.Factor.ID).Where(unchanged(before.Factor)...)
		switch c.trigger {
		case factorStart:
			update.SetGeneration(before.Factor.Generation + 1).SetSecretEncrypted(c.sealedSecret).
				ClearConfirmedAt().SetLastStep(0)
		case factorConfirm:
			update.SetConfirmedAt(c.at).SetLastStep(c.step)
		case factorVerify:
			if c.step > 0 {
				update.SetLastStep(c.step)
			} else {
				update.SetLastStep(before.Factor.LastStep)
			}
		case factorRegenerate:
			update.SetLastStep(before.Factor.LastStep)
		case factorDisable, factorReset:
			update.ClearSecretEncrypted().ClearConfirmedAt().SetLastStep(0)
		}
		saved, err := update.Save(ctx)
		if ent.IsNotFound(err) {
			return false, nil
		}
		out.Factor = saved
		return err == nil, err
	}})
	if err := machine.FireCtx(ctx, c.trigger); err != nil {
		return outcome{}, err
	}
	return out, nil
}

// writeFirst is the change of a User who never enrolled: Start creates the factor row with the
// first enrollment identity (false when a concurrent first Start got there first), and a Reset
// (of a User who holds only Passkeys) has no factor row to write.
func writeFirst(ctx context.Context, tx *ent.Client, userID int64, c change, out *outcome) (bool, error) {
	if c.trigger != factorStart {
		return true, nil
	}
	id, err := tx.SecondFactor.Create().
		SetUserID(userID).
		SetGeneration(1).
		SetSecretEncrypted(c.sealedSecret).
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

// unchanged is the condition the factor row is still before: its enrollment identity, secret,
// confirmation and accepted step. Every change of the row is a compare-and-swap on it, so a stale
// operation never acts on a newer factor.
func unchanged(before *ent.SecondFactor) []predicate.SecondFactor {
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
