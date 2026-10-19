package accounts

import (
	"context"
	"errors"
	"strconv"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
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

// The role decisions below are deliberately separate named functions, not one
// shared predicate: they may diverge, and EE refines them with finer-grained RBAC. Every
// caller (handlers, OAuth connection consent) goes through them; none keeps a copy.

// CanManageMembers reports whether role may change or remove Memberships and manage
// Invitations. Owner and admin may; member may not.
func CanManageMembers(role membership.Role) bool {
	return role == membership.RoleOwner || role == membership.RoleAdmin
}

// CanManageTokens reports whether role may mint or revoke API tokens. A token is a
// standing credential for the whole Workspace, so it is an owner/admin action; OAuth
// connection consent mints one too and takes the same decision.
func CanManageTokens(role membership.Role) bool {
	return role == membership.RoleOwner || role == membership.RoleAdmin
}

// CanManageIntegrations reports whether role may create, edit or delete a sending-provider
// Integration. It holds the Workspace's SMTP password or SES secret key and decides where
// mail is sent with it, so it is an owner/admin action. Reading one (secrets redacted) is
// open to every member.
func CanManageIntegrations(role membership.Role) bool {
	return role == membership.RoleOwner || role == membership.RoleAdmin
}

// CanManageBrandKit reports whether role may save the Workspace's Brand kit and recreate its
// starter Templates (ADR 0040). It is the Workspace's visual identity, so it is an owner-only
// action; every member may read it, because every member edits messages.
func CanManageBrandKit(role membership.Role) bool {
	return role == membership.RoleOwner
}

// CanErase reports whether role may erase a Contact (ADR 0021), an irreversible action
// for the accountable roles.
func CanErase(role membership.Role) bool {
	return role == membership.RoleOwner || role == membership.RoleAdmin
}

// CanManageSecondFactor reports whether role may switch the Workspace's Two-factor
// requirement and reset a member's Second factor (ADR 0020). Owner and admin may.
func CanManageSecondFactor(role membership.Role) bool {
	return role == membership.RoleOwner || role == membership.RoleAdmin
}

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
		target, owners, err := lockedTarget(ctx, ts, targetID)
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
			Diff:        map[string]any{"role": map[string]any{"from": string(target.Role), "to": string(desired)}},
		})
	})
	return updated, err
}

// RemoveMembership removes the Membership targetID from the Workspace s on behalf of a
// caller holding callerRole, under the same Owner invariants and locking as
// ChangeMembershipRole. A target outside the Workspace is an ent NotFound error.
func (a *Accounts) RemoveMembership(ctx context.Context, s *ent.Scoped, callerRole membership.Role, targetID int64) error {
	return a.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		target, owners, err := lockedTarget(ctx, ts, targetID)
		if err != nil {
			return err
		}
		// Removal is a demotion to no Role at all: it is never the owner Role.
		if err := checkOwnerChange(callerRole, target, owners, ""); err != nil {
			return err
		}
		return ts.Membership().DeleteOneID(target.ID).Exec(ctx)
	})
}

// lockedTarget locks the Workspace's owner rows, then loads the target Membership with its
// User and returns the count of owners. Locking first serializes concurrent Owner changes:
// the second waits, then reads the committed outcome of the first.
func lockedTarget(ctx context.Context, ts *ent.Scoped, targetID int64) (*ent.Membership, int, error) {
	ownerIDs, err := ts.Membership().Query().
		Where(membership.RoleEQ(membership.RoleOwner)).
		ForUpdate().
		IDs(ctx)
	if err != nil {
		return nil, 0, err
	}
	target, err := ts.Membership().Query().
		Where(membership.ID(targetID)).
		WithUser().
		Only(ctx)
	if err != nil {
		return nil, 0, err
	}
	return target, len(ownerIDs), nil
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
