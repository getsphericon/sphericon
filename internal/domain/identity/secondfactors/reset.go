package secondfactors

import (
	"context"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/domain/identity/passkeys"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// ErrNothingToReset refuses a reset of a member who has neither an active or pending Second factor
// nor a Passkey, so it has nothing to clear. It refuses the `member` the reset names.
var ErrNothingToReset = validation.RefuseField(memberships.FieldMember, "secondfactor: the member has no second factor or passkey", i18n.ErrorsSecondFactorResetNothing)

// Caller is the member of the Workspace who resets another's Second factor: their User, their Role
// there and the actor the reset is recorded by.
type Caller struct {
	UserID int64
	Role   membership.Role
	Actor  events.Actor
}

// Reset is the Second factor reset (GLOSSARY, ADR 0020): it clears the Second factor, Recovery
// codes and Passkeys of the member whose Membership is membershipID in the Workspace behind s, so
// they can enroll again after losing both their authenticator and their codes, and bumps their
// session epoch, ending every session of theirs. The caller may reset them as
// memberships.SecondFactorResetTarget decides (owners:manage for an Owner's, never their own), read
// in the reset's transaction. It records `user.second_factor_reset` by the caller's actor in that
// Workspace only. ErrNothingToReset without an active or pending factor or a Passkey (nothing
// changes).
func (m *Module) Reset(ctx context.Context, s *ent.Scoped, membershipID int64, caller Caller) error {
	return m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		target, err := memberships.SecondFactorResetTarget(ctx, tx.Scoped(s.WorkspaceID()), caller.UserID, caller.Role, membershipID)
		if err != nil {
			return err
		}
		u, err := resetIn(ctx, tx, target.UserID)
		if err != nil {
			return err
		}
		return users.RecordActionOnUserIn(ctx, pub, caller.Actor, u, events.ActionUserSecondFactorReset, nil, s.WorkspaceID())
	})
}

// ResetByStaff is Reset from the staff member command, of the User with this login email
// (whatever its case; an unknown one fails as `user "<email>"`): the platform Staff member `by`
// (shown to customers as "Sphericon staff") is recorded in every Workspace the User holds a
// Membership in.
func (m *Module) ResetByStaff(ctx context.Context, email, by string) error {
	actor := events.Actor{Kind: events.ActorStaff, ID: by}
	return m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		target, err := users.UserByEmailIn(ctx, tx, email)
		if err != nil {
			return fmt.Errorf("user %q: %w", email, err)
		}
		u, err := resetIn(ctx, tx, target.ID)
		if err != nil {
			return err
		}
		return users.RecordActionOnUser(ctx, tx, pub, actor, u, events.ActionUserSecondFactorReset, nil)
	})
}

// resetIn clears the User's factor (a pending enrollment included), Recovery codes and Passkeys on
// the transaction tx and returns the User with its session epoch bumped. A User without a factor
// row (one who holds only Passkeys) still has the epoch bumped. The factor's move comes first, the
// lock order of every change of the factor: the User row, the factor row, then the codes.
func resetIn(ctx context.Context, tx *ent.Client, userID int64) (*ent.User, error) {
	before, err := snapshotIn(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	held, err := passkeys.HeldIn(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	if stateOf(before.Factor) == factorNone && !held {
		return nil, ErrNothingToReset
	}
	removed, err := remove(ctx, tx, before, factorReset)
	if err != nil {
		return nil, err
	}
	if err := passkeys.RemoveAllIn(ctx, tx, userID); err != nil {
		return nil, err
	}
	return removed.User, nil
}
