// Package secondfactorsunchecked holds the secondfactors module's unchecked removal (ADR 0110): a
// User's Second factor is cleared without its code only in a Second factor reset, which also removes
// the User's Passkeys, checks there is something to clear and records who reset, so the check is
// the use case internal/usecase/secondfactorreset's (ADR 0020); only a use case may import this
// package. The secondfactors module does not import it; its lifecycle (secondfactors.Machine)
// declares the reset move, which is written here.
package secondfactorsunchecked

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/recoverycode"
	"github.com/getsphericon/sphericon/ent/secondfactor"
	"github.com/getsphericon/sphericon/internal/domain/identity/secondfactors"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
)

// ResetIn clears the User's Second factor (a pending enrollment included) and Recovery codes on the
// transaction tx and bumps the session epoch, ending every session of theirs, and returns the
// stored User. It checks no authority and nothing to clear: the Second factor reset (the use case
// internal/usecase/secondfactorreset) has checked both. A User without a factor row (one who holds
// only Passkeys) still has the epoch bumped. The move is a compare-and-swap on the User and the
// factor row it read, like every change of the factor.
func ResetIn(ctx context.Context, tx *ent.Client, userID int64) (*ent.User, error) {
	before, err := secondfactors.SnapshotIn(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	user := before.User
	reset := secondfactors.Change{Trigger: secondfactors.FactorReset}
	machine := secondfactors.Machine(before.Factor, reset, func(ctx context.Context, _ secondfactors.State) (bool, error) {
		saved, bumped, err := users.BumpSessionEpochFromIn(ctx, tx, before.User)
		if err != nil || !bumped {
			return false, err
		}
		user = saved
		if before.Factor == nil {
			return true, nil
		}
		n, err := tx.SecondFactor.Update().
			Where(secondfactor.ID(before.Factor.ID)).
			Where(secondfactors.Unchanged(before.Factor)...).
			ClearSecretEncrypted().ClearConfirmedAt().SetLastStep(0).
			Save(ctx)
		return n == 1, err
	})
	if err := machine.FireCtx(ctx, reset.Trigger); err != nil {
		return nil, err
	}
	if _, err := tx.RecoveryCode.Delete().Where(recoverycode.UserID(userID)).Exec(ctx); err != nil {
		return nil, err
	}
	return user, nil
}
