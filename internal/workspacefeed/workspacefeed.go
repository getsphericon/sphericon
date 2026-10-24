// Package workspacefeed keeps the change position behind the service feed of
// Workspaces (ADR 0009). A Workspace takes the next position whenever it is created,
// an owner is added, removed or transferred, an owner's email or verification
// changes, or it is deleted, restored or erased (ADR 0043). The service endpoint reads Workspaces in the order of that position
// (ee/staff), so what a caller saved as its cursor always names a place in the
// order of changes.
//
// Two ent hooks keep it so no write path can forget it. Hook runs on every transaction
// client (the event bus's): the change and its position commit together, in the order
// the positions were taken. Guard runs on the plain client (db.NewEntClient) and refuses
// a change to the feed outside a transaction, where the position would be a separate
// statement that a crash loses and a concurrent change commits out of order. The hook
// reads Memberships of any Workspace to find the one a change belongs to, which is why
// this package is on the closed list of raw-client packages in docs/architecture.md.
package workspacefeed

import (
	"context"
	"errors"
	"fmt"

	"entgo.io/ent"

	entclient "github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/postgres"
)

// lockKey is the advisory lock that makes positions commit in the order they are taken:
// it is held to the end of the transaction, so a later position never commits first
// and a reader that passed a position cannot miss a Workspace committed below it.
const lockKey = 7_204_001

// ErrNeedsTransaction is what Guard returns for a change to the feed made on a client
// that is not bound to a transaction.
var ErrNeedsTransaction = errors.New("workspace feed change outside a transaction")

// Guard is the ent hook for a client that is not bound to a transaction: it refuses
// every mutation that Hook would give a position to. Such a change goes through the
// event bus's transaction (events.Bus.WithinTx).
func Guard(next ent.Mutator) ent.Mutator {
	return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
		if changesFeed(m) {
			return nil, ErrNeedsTransaction
		}
		return next.Mutate(ctx, m)
	})
}

// changesFeed reports whether m can change what the feed reports, before looking at
// any row: Hook narrows it to the Workspaces an owner change actually touches.
func changesFeed(m ent.Mutation) bool {
	switch mu := m.(type) {
	case *entclient.WorkspaceMutation:
		return mu.Op().Is(ent.OpCreate) || changesDeletion(mu)
	case *entclient.MembershipMutation:
		if mu.Op().Is(ent.OpCreate) {
			role, ok := mu.Role()
			return ok && role == membership.RoleOwner
		}
		if mu.Op().Is(ent.OpUpdate | ent.OpUpdateOne) {
			_, changesRole := mu.Role()
			return changesRole
		}
		return mu.Op().Is(ent.OpDelete | ent.OpDeleteOne)
	case *entclient.UserMutation:
		if !mu.Op().Is(ent.OpUpdate | ent.OpUpdateOne) {
			return false
		}
		_, email := mu.Email()
		_, verified := mu.EmailVerifiedAt()
		return email || verified || mu.EmailVerifiedAtCleared()
	}
	return false
}

// changesDeletion reports whether m is an update that sets or clears discarded_at or
// erased_at: the deletion state the feed reports (ADR 0043).
func changesDeletion(m *entclient.WorkspaceMutation) bool {
	if !m.Op().Is(ent.OpUpdate | ent.OpUpdateOne) {
		return false
	}
	_, deleted := m.DiscardedAt()
	_, erased := m.ErasedAt()
	return deleted || erased || m.DiscardedAtCleared() || m.ErasedAtCleared()
}

// Hook is the ent hook to install on a client bound to a transaction (Client.Use). It
// is a no-op for every mutation that cannot change what the feed reports.
func Hook(next ent.Mutator) ent.Mutator {
	return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
		switch mu := m.(type) {
		case *entclient.WorkspaceMutation:
			if changesDeletion(mu) {
				ids, err := mu.IDs(ctx)
				if err != nil {
					return nil, err
				}
				v, err := next.Mutate(ctx, m)
				if err != nil || len(ids) == 0 {
					return v, err
				}
				return v, touch(ctx, mu.Client(), ids...)
			}
			if !mu.Op().Is(ent.OpCreate) {
				return next.Mutate(ctx, m)
			}
			v, err := next.Mutate(ctx, m)
			if err != nil {
				return v, err
			}
			if id, ok := mu.ID(); ok {
				err = touch(ctx, mu.Client(), id)
			}
			return v, err
		case *entclient.MembershipMutation:
			return mutateMembership(ctx, next, mu)
		case *entclient.UserMutation:
			return mutateUser(ctx, next, mu)
		}
		return next.Mutate(ctx, m)
	})
}

func mutateMembership(ctx context.Context, next ent.Mutator, m *entclient.MembershipMutation) (ent.Value, error) {
	c := m.Client()
	var affected []int64
	switch {
	case m.Op().Is(ent.OpCreate):
		// A new Membership names its Workspace and Role itself.
		v, err := next.Mutate(ctx, m)
		if err != nil {
			return v, err
		}
		if role, ok := m.Role(); ok && role == membership.RoleOwner {
			if ws, ok := m.WorkspaceID(); ok {
				err = touch(ctx, c, ws)
			}
		}
		return v, err
	case m.Op().Is(ent.OpDelete | ent.OpDeleteOne):
		// Only the removal of an owner is a change, and the row is gone afterwards.
		ids, err := m.IDs(ctx)
		if err != nil {
			return nil, err
		}
		affected, err = workspacesOf(ctx, c.Membership.Query().
			Where(membership.IDIn(ids...), membership.RoleEQ(membership.RoleOwner)))
		if err != nil {
			return nil, err
		}
	case m.Op().Is(ent.OpUpdate | ent.OpUpdateOne):
		if _, changesRole := m.Role(); !changesRole {
			return next.Mutate(ctx, m)
		}
		ids, err := m.IDs(ctx)
		if err != nil {
			return nil, err
		}
		affected, err = workspacesOf(ctx, c.Membership.Query().Where(membership.IDIn(ids...)))
		if err != nil {
			return nil, err
		}
	}
	v, err := next.Mutate(ctx, m)
	if err != nil || len(affected) == 0 {
		return v, err
	}
	return v, touch(ctx, c, affected...)
}

func mutateUser(ctx context.Context, next ent.Mutator, m *entclient.UserMutation) (ent.Value, error) {
	if !m.Op().Is(ent.OpUpdate | ent.OpUpdateOne) {
		return next.Mutate(ctx, m)
	}
	_, email := m.Email()
	_, verified := m.EmailVerifiedAt()
	cleared := m.EmailVerifiedAtCleared()
	if !email && !verified && !cleared {
		return next.Mutate(ctx, m)
	}
	c := m.Client()
	ids, err := m.IDs(ctx)
	if err != nil {
		return nil, err
	}
	v, err := next.Mutate(ctx, m)
	if err != nil {
		return v, err
	}
	owned, err := workspacesOf(ctx, c.Membership.Query().
		Where(membership.UserIDIn(ids...), membership.RoleEQ(membership.RoleOwner)))
	if err != nil || len(owned) == 0 {
		return v, err
	}
	return v, touch(ctx, c, owned...)
}

// workspacesOf is the Workspaces the Memberships of q belong to.
func workspacesOf(ctx context.Context, q *entclient.MembershipQuery) ([]int64, error) {
	rows, err := q.All(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.WorkspaceID
	}
	return ids, nil
}

// touch gives each Workspace the next position, all of them the same one. The lock is
// taken first and held to the end of the surrounding transaction.
func touch(ctx context.Context, c *entclient.Client, workspaceIDs ...int64) error {
	if err := postgres.LockTx(ctx, c, lockKey); err != nil {
		return fmt.Errorf("workspace feed lock: %w", err)
	}
	// The lock keeps the position unique: no other transaction can take the maximum
	// between this read and the update.
	top, err := c.Workspace.Query().Aggregate(entclient.Max(workspace.FieldFeedPosition)).Int(ctx)
	if err != nil {
		return fmt.Errorf("workspace feed position: %w", err)
	}
	if err := c.Workspace.Update().Where(workspace.IDIn(workspaceIDs...)).SetFeedPosition(int64(top) + 1).Exec(ctx); err != nil {
		return fmt.Errorf("workspace feed position: %w", err)
	}
	return nil
}
