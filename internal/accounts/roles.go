package accounts

import (
	"context"
	"errors"
	"strconv"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/events"
)

// The Owner invariants of a Role (GLOSSARY "Role"): a Workspace always keeps at least one
// owner, and only an owner grants the owner Role or changes or removes another owner's
// Membership. Surfaces map ErrOwnerOnly to 403 and ErrLastOwner to 422.
var (
	// ErrOwnerOnly is returned when a caller who is not an owner grants the owner Role or
	// changes or removes an owner's Membership.
	ErrOwnerOnly = errors.New("accounts: only an owner may grant the owner role or change an owner")
	// ErrLastOwner is returned when the change would leave the Workspace without an owner.
	ErrLastOwner = errors.New("accounts: a workspace keeps at least one owner")
)

// ChangeMembershipRole sets the Role of the Membership targetID in the Workspace s on
// behalf of a caller holding callerRole, and records `membership.update` with the role
// change as an Audit entry in the same transaction (ADR 0022): a rolled-back change
// leaves no entry, a committed one cannot lose it. The Owner invariants are checked in
// that transaction while the Workspace's owner rows are locked, so two concurrent
// changes cannot leave the Workspace ownerless. The returned Membership carries its User.
// A target outside the Workspace is an ent NotFound error.
func (a *Accounts) ChangeMembershipRole(ctx context.Context, s *ent.Scoped, actor events.Actor, callerRole membership.Role, targetID int64, desired membership.Role) (*ent.Membership, error) {
	var updated *ent.Membership
	err := a.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		target, owners, err := lockedTarget(ctx, ts, membership.ID(targetID))
		if err != nil {
			return err
		}
		if err := checkOwnerChange(callerRole, target, owners, desired); err != nil {
			return err
		}
		m, err := ts.Membership().UpdateOneID(target.ID).SetRole(desired).Save(ctx)
		if err != nil {
			return err
		}
		m.Edges.User = target.Edges.User
		updated = m
		return events.RecordAudit(ctx, pub, &events.AuditEntry{
			WorkspaceID: s.WorkspaceID(),
			Actor:       actor,
			Action:      events.ActionMembershipUpdate,
			TargetType:  "membership",
			TargetID:    strconv.FormatInt(target.ID, 10),
			TargetName:  target.Edges.User.Name,
			Diff:        map[string]any{membership.FieldRole: events.Change(string(target.Role), string(desired))},
		})
	})
	return updated, err
}

// RemoveMembership removes the Membership targetID from the Workspace s on behalf of a
// caller holding callerRole, under the same Owner invariants and locking as
// ChangeMembershipRole, and records `membership.delete` with actor as an Audit entry in the
// same transaction. A target outside the Workspace is an ent NotFound error.
func (a *Accounts) RemoveMembership(ctx context.Context, s *ent.Scoped, actor events.Actor, callerRole membership.Role, targetID int64) error {
	return a.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		target, owners, err := lockedTarget(ctx, ts, membership.ID(targetID))
		if err != nil {
			return err
		}
		return removeLocked(ctx, ts, pub, actor, callerRole, target, owners)
	})
}

// Leave removes User userID's own Membership in the Workspace s (Leaving, ADR 0050) in a
// transaction of its own. See LeaveIn for the rules.
func (a *Accounts) Leave(ctx context.Context, s *ent.Scoped, userID int64) error {
	return a.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		return LeaveIn(ctx, ts, pub, userID)
	})
}

// LeaveIn removes User userID's own Membership through the scope ts, inside the transaction
// ts is bound to (ts comes from Bus.WithinScopedTx, or from tx.Scoped in a caller's own
// transaction, with pub that transaction's publisher). The User is the actor of the
// `membership.delete` Audit entry, named by the User row. It locks the Workspace's Owner
// rows first, like every Owner change, and refuses the only Owner with ErrLastOwner.
// A User with no Membership in the Workspace is an ent NotFound error. User deletion
// (ADR 0050) calls it once per Workspace in its own transaction.
func LeaveIn(ctx context.Context, ts *ent.Scoped, pub events.Publisher, userID int64) error {
	target, owners, actor, err := lockedSelf(ctx, ts, userID)
	if err != nil {
		return err
	}
	// The caller is the member themself: their own Role is the caller's Role.
	return removeLocked(ctx, ts, pub, actor, target.Role, target, owners)
}

// leaveDoomedWorkspaceIn removes User userID's own Membership like LeaveIn but lets the only
// Owner go too, for User deletion of a Workspace that is in its deletion window (ADR 0050):
// nobody is left to hand it to, and the Workspace is erased on schedule.
func leaveDoomedWorkspaceIn(ctx context.Context, ts *ent.Scoped, pub events.Publisher, userID int64) error {
	target, _, actor, err := lockedSelf(ctx, ts, userID)
	if err != nil {
		return err
	}
	return deleteLocked(ctx, ts, pub, actor, target)
}

// lockedSelf locks the Workspace's Owner rows, then loads User userID's own Membership with
// its User, the count of Owners, and the User as the actor of the Audit entry.
func lockedSelf(ctx context.Context, ts *ent.Scoped, userID int64) (target *ent.Membership, owners int, actor events.Actor, err error) {
	target, owners, err = lockedTarget(ctx, ts, membership.UserID(userID))
	if err != nil {
		return nil, 0, events.Actor{}, err
	}
	actor = events.Actor{Kind: events.ActorUser, ID: strconv.FormatInt(userID, 10), Name: target.Edges.User.Name}
	return target, owners, actor, nil
}

// removeLocked applies the Owner invariants to removing target (whose Workspace's owner
// rows the caller holds locked), deletes it and records the Audit entry.
func removeLocked(ctx context.Context, ts *ent.Scoped, pub events.Publisher, actor events.Actor, callerRole membership.Role, target *ent.Membership, owners int) error {
	// Removal is a demotion to no Role at all: it is never the owner Role.
	if err := checkOwnerChange(callerRole, target, owners, ""); err != nil {
		return err
	}
	return deleteLocked(ctx, ts, pub, actor, target)
}

// deleteLocked deletes target and records the Audit entry, with no invariant checked.
func deleteLocked(ctx context.Context, ts *ent.Scoped, pub events.Publisher, actor events.Actor, target *ent.Membership) error {
	if err := ts.Membership().DeleteOneID(target.ID).Exec(ctx); err != nil {
		return err
	}
	return events.RecordAudit(ctx, pub, &events.AuditEntry{
		WorkspaceID: ts.WorkspaceID(),
		Actor:       actor,
		Action:      events.ActionMembershipDelete,
		TargetType:  "membership",
		TargetID:    strconv.FormatInt(target.ID, 10),
		TargetName:  target.Edges.User.Name,
		Diff:        map[string]any{membership.FieldRole: events.Change(string(target.Role), nil)},
	})
}

// lockedTarget locks the Workspace's owner rows, then loads the one Membership matching
// target with its User and returns the count of owners. Locking first serializes
// concurrent Owner changes: the second waits, then reads the committed outcome of the first.
func lockedTarget(ctx context.Context, ts *ent.Scoped, target predicate.Membership) (*ent.Membership, int, error) {
	ownerIDs, err := LockOwners(ctx, ts)
	if err != nil {
		return nil, 0, err
	}
	m, err := ts.Membership().Query().
		Where(target).
		WithUser().
		Only(ctx)
	if err != nil {
		return nil, 0, err
	}
	return m, len(ownerIDs), nil
}

// checkOwnerChange applies the Owner invariants to turning target into desired ("" when
// the Membership is removed).
func checkOwnerChange(callerRole membership.Role, target *ent.Membership, owners int, desired membership.Role) error {
	if callerRole != membership.RoleOwner && (desired == membership.RoleOwner || target.Role == membership.RoleOwner) {
		return ErrOwnerOnly
	}
	if target.Role == membership.RoleOwner && desired != membership.RoleOwner && owners <= 1 {
		return ErrLastOwner
	}
	return nil
}
