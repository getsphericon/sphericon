package secondfactors

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
)

// EnrolledIn reports, on the transaction tx, whether the User has a Second factor, active or a
// pending enrollment: what a Second factor reset would clear besides the Passkeys.
func EnrolledIn(ctx context.Context, tx *ent.Client, userID int64) (bool, error) {
	f, err := factorOf(ctx, tx, userID)
	return stateOf(f) != FactorNone, err
}

// ResetIn clears the User's Second factor (a pending enrollment included) and Recovery codes on the
// transaction tx and bumps the session epoch, ending every session of theirs, and returns the
// stored User. It checks no authority and nothing to clear: only the Second factor reset (the use
// case internal/usecase/secondfactorreset, which also removes the User's Passkeys and records who
// reset) calls it, after it has checked both (ADR 0020, ADR 0110). A User without a factor row (one
// who holds only Passkeys) still has the epoch bumped.
func ResetIn(ctx context.Context, tx *ent.Client, userID int64) (*ent.User, error) {
	before, err := snapshotIn(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	moved, err := remove(ctx, tx, before, FactorReset)
	if err != nil {
		return nil, err
	}
	return moved.User, nil
}
