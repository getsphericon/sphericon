package accounts

import (
	"slices"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/validation"
)

// The Permission type and its values are generated from the contract's Permission enum
// (permissions_gen.go); the table below says which Roles hold each.

// grant is one row of the Role table: a Permission, the Roles that hold it, and the refusal a
// Role without it is answered with (403 with code permission_required on every surface).
type grant struct {
	permission Permission
	roles      []membership.Role
	refusal    *validation.RefusalError
}

var (
	ownerAndAdmin = []membership.Role{membership.RoleOwner, membership.RoleAdmin}
	ownerOnly     = []membership.Role{membership.RoleOwner}
)

// row is the grant of p to roles, refused with the message key.
func row(p Permission, key i18n.ID, roles []membership.Role) grant {
	return grant{permission: p, roles: roles, refusal: validation.Forbidden("accounts: the role lacks the permission "+string(p), key)}
}

// grants is the one table from Role to Permissions (the Gate of ADR 0061): a Role is the set of
// Permissions whose row names it. Nothing else holds a copy of it; the client reads a
// Membership's Permissions from its Workspace. An owner differs from an admin by ownership and
// the irreversible; a member holds none of them.
var grants = []grant{
	row(PermMembersManage, i18n.ErrorsPermissionMembersManage, ownerAndAdmin),
	row(PermAuditRead, i18n.ErrorsPermissionAuditRead, ownerAndAdmin),
	row(PermAuditConfigure, i18n.ErrorsPermissionAuditConfigure, ownerAndAdmin),
	row(PermTokensManage, i18n.ErrorsPermissionTokensManage, ownerAndAdmin),
	row(PermIntegrationsManage, i18n.ErrorsPermissionIntegrationsManage, ownerAndAdmin),
	row(PermWebhooksManage, i18n.ErrorsPermissionWebhooksManage, ownerAndAdmin),
	row(PermSendingDomainsManage, i18n.ErrorsPermissionSendingDomainsManage, ownerAndAdmin),
	row(PermContactsErase, i18n.ErrorsPermissionContactsErase, ownerAndAdmin),
	row(PermContactsExport, i18n.ErrorsPermissionContactsExport, ownerAndAdmin),
	row(PermSuppressionsManage, i18n.ErrorsPermissionSuppressionsManage, ownerAndAdmin),
	row(PermSecondFactorManage, i18n.ErrorsPermissionSecondFactorManage, ownerAndAdmin),
	row(PermWorkspaceUpdate, i18n.ErrorsPermissionWorkspaceUpdate, ownerAndAdmin),
	row(PermWorkspaceTimezone, i18n.ErrorsPermissionWorkspaceTimezone, ownerAndAdmin),
	row(PermRevenueConfigure, i18n.ErrorsPermissionRevenueConfigure, ownerAndAdmin),
	row(PermBrandKitManage, i18n.ErrorsPermissionBrandKitManage, ownerAndAdmin),
	row(PermJoinDomainsManage, i18n.ErrorsPermissionJoinDomainsManage, ownerAndAdmin),
	row(PermTrackingDomainsManage, i18n.ErrorsPermissionTrackingDomainsManage, ownerAndAdmin),
	row(PermHoldsResolve, i18n.ErrorsPermissionHoldsResolve, ownerAndAdmin),
	row(PermAccountLinkOpen, i18n.ErrorsPermissionAccountLinkOpen, ownerOnly),
	row(PermWorkspaceDelete, i18n.ErrorsPermissionWorkspaceDelete, ownerOnly),
	row(PermOwnersManage, i18n.ErrorsPermissionOwnersManage, ownerOnly),
	row(PermReleaseRead, i18n.ErrorsPermissionReleaseRead, ownerOnly),
}

// Allows reports whether role holds the Permission p. A Permission outside the table is held
// by no Role. It is the seam an Enterprise implementation of finer Roles would replace.
func Allows(role membership.Role, p Permission) bool {
	g, ok := grantOf(p)
	return ok && slices.Contains(g.roles, role)
}

// Require is nil when role holds the Permission p, and otherwise p's refusal: a sentinel of kind
// validation.KindForbidden, which every surface answers 403 with code permission_required. A
// caller returns it as it is.
func Require(role membership.Role, p Permission) error {
	g, ok := grantOf(p)
	if !ok {
		return errPermissionUnknown
	}
	if slices.Contains(g.roles, role) {
		return nil
	}
	return g.refusal
}

// Refusal is p's refusal (403 permission_required on every surface), for an operation that found
// no Membership holding p, such as one that asks HoldersOf across Workspaces. A Permission outside
// the table is refused like Require refuses it.
func Refusal(p Permission) error {
	g, ok := grantOf(p)
	if !ok {
		return errPermissionUnknown
	}
	return g.refusal
}

// GrantRequires names, for each of scopes that needs one, the Permission a User must hold to grant
// it (GrantPermissions, declared on each grantable scope in the contract): what the consent screen
// shows beside a scope.
func GrantRequires(scopes []string) map[string]Permission {
	needed := map[string]Permission{}
	for _, scope := range scopes {
		if p, ok := GrantPermissions[scope]; ok {
			needed[scope] = p
		}
	}
	return needed
}

// MayGrant is nil when role holds the Permission granting each of scopes needs, and otherwise the
// refusal of the first Permission it lacks, in the order of scopes (ADR 0061). It bounds what a
// User grants a Connection when they approve it, and what the Connection may still do on each
// request after the User's Role changed.
func MayGrant(role membership.Role, scopes []string) error {
	for _, scope := range scopes {
		p, ok := GrantPermissions[scope]
		if !ok {
			continue
		}
		if err := Require(role, p); err != nil {
			return err
		}
	}
	return nil
}

// errPermissionUnknown refuses a Permission outside the table, which no Role holds.
var errPermissionUnknown = validation.Forbidden("accounts: the permission is unknown", i18n.ErrorsPermissionUnknown)

// grantOf is the row of the Permission p, false when the table has none.
func grantOf(p Permission) (grant, bool) {
	return lo.Find(grants, func(g grant) bool { return g.permission == p })
}

// Permissions is every Permission role holds, in the order of the access table.
func Permissions(role membership.Role) []Permission {
	held := lo.Filter(grants, func(g grant, _ int) bool { return Allows(role, g.permission) })
	return lo.Map(held, func(g grant, _ int) Permission { return g.permission })
}

// HoldersOf is the Memberships whose Role holds the Permission p: the query form of Allows, for an
// operation that finds its Workspace through the caller's Membership rather than by a workspaceId
// (restoring a deleted Workspace). A Permission outside the table has no row and so no Roles,
// which ent renders as a false predicate: no Membership holds it.
func HoldersOf(p Permission) predicate.Membership {
	g, _ := grantOf(p)
	return membership.RoleIn(g.roles...)
}
