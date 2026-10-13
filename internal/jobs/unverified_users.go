package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/user"
)

// unverifiedUsersBatch bounds the rows one DELETE removes so a large backlog never
// holds a long lock.
const unverifiedUsersBatch = 500

// unverifiedUsersHourUTC is the off-peak hour the Unverified User prune runs at.
const unverifiedUsersHourUTC = 4

// PruneUnverifiedUsersArgs is the daily tick that deletes stale Unverified Users
// (ADR 0030).
type PruneUnverifiedUsersArgs struct{}

func (PruneUnverifiedUsersArgs) Kind() string { return "unverified_users_prune" }

// PruneUnverifiedUsersWorker frees the addresses held by registrations whose verify
// link was never used. Users are not Workspace-scoped, so it holds the raw client; a
// zero retention disables it.
type PruneUnverifiedUsersWorker struct {
	river.WorkerDefaults[PruneUnverifiedUsersArgs]
	ent       *ent.Client
	retention time.Duration
}

func (w *PruneUnverifiedUsersWorker) Work(ctx context.Context, _ *river.Job[PruneUnverifiedUsersArgs]) error {
	deleted, err := PruneUnverifiedUsers(ctx, w.ent, w.retention, unverifiedUsersBatch)
	if err != nil {
		return err
	}
	if deleted > 0 {
		slog.InfoContext(ctx, "stale unverified users pruned", "deleted", deleted)
	}
	return nil
}

// PruneUnverifiedUsers deletes Users that have no Verified email and no Membership
// and were created more than retention ago, at most batch rows per DELETE, and returns
// how many went. A zero retention deletes nothing. Invitations reference an email, not
// a User, so a pending one to a pruned address is untouched.
func PruneUnverifiedUsers(ctx context.Context, client *ent.Client, retention time.Duration, batch int) (int64, error) {
	if retention <= 0 {
		return 0, nil
	}
	cutoff := time.Now().Add(-retention)
	var total int64
	for {
		ids, err := client.User.Query().
			Where(user.EmailVerifiedAtIsNil(), user.CreatedAtLT(cutoff), user.Not(user.HasMemberships())).
			Order(user.ByID()).
			Limit(batch).
			IDs(ctx)
		if err != nil {
			return total, err
		}
		if len(ids) == 0 {
			return total, nil
		}
		n, err := client.User.Delete().
			Where(user.IDIn(ids...), user.EmailVerifiedAtIsNil(), user.Not(user.HasMemberships())).
			Exec(ctx)
		if err != nil {
			return total, err
		}
		total += int64(n)
		if len(ids) < batch {
			return total, nil
		}
	}
}
