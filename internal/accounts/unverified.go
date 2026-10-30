package accounts

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/internal/events"
)

// PruneUnverified deletes Users that have no Verified email and no Membership and were
// created more than retention ago, at most batch rows per DELETE, and returns how many went.
// A zero retention deletes nothing. Invitations reference an email, not a User, so a pending
// one to a pruned address is untouched (ADR 0030).
func (a *Accounts) PruneUnverified(ctx context.Context, retention time.Duration, batch int) (int64, error) {
	if retention <= 0 {
		return 0, nil
	}
	cutoff := time.Now().Add(-retention)
	stale := func(extra predicate.User) []predicate.User {
		return []predicate.User{user.EmailVerifiedAtIsNil(), user.Not(user.HasMemberships()), extra}
	}
	var total int64
	for {
		ids, err := a.ent.User.Query().
			Where(stale(user.CreatedAtLT(cutoff))...).
			Order(user.ByID()).
			Limit(batch).
			IDs(ctx)
		if err != nil {
			return total, err
		}
		if len(ids) == 0 {
			return total, nil
		}
		// The deletion leaves a tombstone in the service feed of Users in its own transaction.
		var n int
		err = a.bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
			n, err = tx.User.Delete().Where(stale(user.IDIn(ids...))...).Exec(ctx)
			return err
		})
		if err != nil {
			return total, err
		}
		total += int64(n)
		if len(ids) < batch {
			return total, nil
		}
	}
}
