package secondfactors

import (
	"context"
	"errors"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
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

// Reset is the Second factor reset (GLOSSARY, ADR 0020, ADR 0177): it clears the Second factor,
// Recovery codes and Passkeys of the member whose Membership is membershipID in the Workspace
// behind s, so they can enroll again after losing both their authenticator and their codes, and
// bumps their session epoch, ending every session of theirs. These are the User's, not the
// Workspace's, so the caller may reset them only where they may reset the member in every Workspace
// of theirs, read in the reset's transaction: in this one as
// memberships.Module.SecondFactorResetTarget decides (owners:manage for an Owner's, never their
// own), in the others as memberships.Module.SecondFactorResetElsewhereIn decides
// (memberships.ErrSecondFactorResetElsewhere). The reset is then told as resetAndTellIn tells it.
// ErrNothingToReset without an active or pending factor or a Passkey (nothing changes).
func (m *Module) Reset(ctx context.Context, s *ent.Scoped, membershipID int64, caller Caller) error {
	return m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		target, err := m.members.SecondFactorResetTarget(ctx, tx.Scoped(s.WorkspaceID()), memberships.SecondFactorResetTargetInput{
			CallerID: caller.UserID, CallerRole: caller.Role, TargetID: membershipID,
		})
		if err != nil {
			return err
		}
		err = m.members.SecondFactorResetElsewhereIn(ctx, tx, memberships.SecondFactorResetElsewhereInput{CallerID: caller.UserID, Target: target})
		if err != nil {
			return err
		}
		return m.resetAndTellIn(ctx, tx, pub, target.UserID, caller.Actor)
	})
}

// ResetByStaff is Reset from the staff member command, of the User with this login email
// (whatever its case; an unknown one fails as `user "<email>"`), by the platform Staff member `by`
// (shown to customers as "Sphericon staff"), told as resetAndTellIn tells it.
func (m *Module) ResetByStaff(ctx context.Context, email, by string) error {
	actor := events.Actor{Kind: events.ActorStaff, ID: by}
	return m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		target, err := m.users.UserByEmailIn(ctx, tx, email)
		if err != nil {
			return fmt.Errorf("user %q: %w", email, err)
		}
		return m.resetAndTellIn(ctx, tx, pub, target.ID, actor)
	})
}

// resetAndTellIn resets the User's Second factor on the transaction tx (resetIn) and tells of it:
// `user.second_factor_reset` by actor in the log of every Workspace the User holds a Membership in,
// and events.SecondFactorReset, whose Notice mails the User (ADR 0177).
func (m *Module) resetAndTellIn(ctx context.Context, tx *ent.Client, pub events.Publisher, userID int64, actor events.Actor) error {
	u, err := m.resetIn(ctx, tx, userID)
	if err != nil {
		return err
	}
	if err := m.users.RecordActionOnUser(ctx, tx, users.ActionOnUser{Actor: actor, User: u, Action: events.ActionUserSecondFactorReset}); err != nil {
		return err
	}
	return pub.Publish(ctx, &events.SecondFactorReset{UserID: u.ID, Email: u.Email})
}

// ClearFactorAndPasskeysIn clears whatever Second factor, Recovery codes and Passkeys the User holds on the
// transaction tx, as a reset does, without asking that there be any: the first proof of an address
// drops what was set up before it (ADR 0170). It returns the User as it stands afterwards.
func (m *Module) ClearFactorAndPasskeysIn(ctx context.Context, tx *ent.Client, userID int64) (*ent.User, error) {
	u, err := m.resetIn(ctx, tx, userID)
	if errors.Is(err, ErrNothingToReset) {
		return tx.User.Get(ctx, userID)
	}
	return u, err
}

// resetIn clears the User's factor (a pending enrollment included), Recovery codes and Passkeys on
// the transaction tx and returns the User with its session epoch bumped. A User without a factor
// row (one who holds only Passkeys) still has the epoch bumped. The factor's move comes first, the
// lock order of every change of the factor: the User row, the factor row, then the codes.
func (m *Module) resetIn(ctx context.Context, tx *ent.Client, userID int64) (*ent.User, error) {
	before, err := snapshotIn(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	held, err := m.passkeys.HeldIn(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	if stateOf(before.Factor) == factorNone && !held {
		return nil, ErrNothingToReset
	}
	removed, err := m.remove(ctx, tx, before, factorReset)
	if err != nil {
		return nil, err
	}
	if err := m.passkeys.RemoveAllIn(ctx, tx, userID); err != nil {
		return nil, err
	}
	return removed.User, nil
}
