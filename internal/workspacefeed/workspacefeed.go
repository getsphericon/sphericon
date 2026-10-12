// Package workspacefeed keeps the change position behind the service feed of
// Workspaces (ADR 0009). A Workspace takes the next position whenever it is created,
// an owner is added, removed or transferred, or an owner's email or verification
// changes. The service endpoint reads Workspaces in the order of that position
// (ee/operator), so what a caller saved as its cursor always names a place in the
// order of changes.
//
// It is an ent hook installed on every client that is built (db.NewEntClient, the event
// bus's transaction client), so no write path can forget it. The hook reads Memberships
// of any Workspace to find the one a change belongs to, which is why this package is on
// the closed list of raw-client packages in AGENTS.md.
package workspacefeed

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"entgo.io/ent"

	entclient "github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
)

// lockKey is the advisory lock that makes positions commit in the order they are taken:
// it is held to the end of the transaction, so a later position never commits first
// and a reader that passed a position cannot miss a Workspace committed below it.
const lockKey = 7_204_001

// Hook is the ent hook to install on a client (Client.Use). It is a no-op for every
// mutation that cannot change what the feed reports.
func Hook(next ent.Mutator) ent.Mutator {
	return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
		switch mu := m.(type) {
		case *entclient.WorkspaceMutation:
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
	if _, err := c.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", lockKey); err != nil {
		return fmt.Errorf("workspace feed lock: %w", err)
	}
	args := make([]any, len(workspaceIDs))
	marks := make([]string, len(workspaceIDs))
	for i, id := range workspaceIDs {
		args[i] = id
		marks[i] = "$" + strconv.Itoa(i+1)
	}
	query := "UPDATE workspaces SET feed_position = (SELECT COALESCE(MAX(feed_position), 0) + 1 FROM workspaces)" +
		" WHERE id IN (" + strings.Join(marks, ", ") + ")"
	if _, err := c.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("workspace feed position: %w", err)
	}
	return nil
}
