package accounts

import (
	"slices"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/internal/validation"
)

// Permission is a named action in a Workspace that a Role allows (GLOSSARY "Permission",
// ADR 0061), written `resource:verb`. Its values are the contract's Permission enum.
type Permission string

// The Permissions of the access table of ADR 0061, in its order.
const (
	PermMembersManage        Permission = "members:manage"
	PermAuditRead            Permission = "audit:read"
	PermAuditConfigure       Permission = "audit:configure"
	PermTokensManage         Permission = "tokens:manage"
	PermIntegrationsManage   Permission = "integrations:manage"
	PermWebhooksManage       Permission = "webhooks:manage"
	PermSendingDomainsManage Permission = "sending_domains:manage"
	PermContactsErase        Permission = "contacts:erase"
	PermContactsExport       Permission = "contacts:export"
	PermSuppressionsManage   Permission = "suppressions:manage"
	PermSecondFactorManage   Permission = "second_factor:manage"
	PermWorkspaceUpdate      Permission = "workspace:update"
	PermWorkspaceTimezone    Permission = "workspace:timezone"
	PermRevenueConfigure     Permission = "revenue:configure"
	PermBrandKitManage       Permission = "brand_kit:manage"
	PermJoinDomainsManage    Permission = "join_domains:manage"
	PermAccountLinkOpen      Permission = "account_link:open"
	PermWorkspaceDelete      Permission = "workspace:delete"
)

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

// row is the grant of p to roles, refused with the i18n message key.
func row(p Permission, key string, roles []membership.Role) grant {
	return grant{permission: p, roles: roles, refusal: validation.Forbidden("accounts: the role lacks the permission "+string(p), key)}
}

// grants is the one table from Role to Permissions (the Gate of ADR 0061): a Role is the set of
// Permissions whose row names it. Nothing else holds a copy of it; the client reads a
// Membership's Permissions from its Workspace. An owner differs from an admin by ownership and
// the irreversible; a member holds none of them.
var grants = []grant{
	row(PermMembersManage, "errors.permission.members_manage", ownerAndAdmin),
	row(PermAuditRead, "errors.permission.audit_read", ownerAndAdmin),
	row(PermAuditConfigure, "errors.permission.audit_configure", ownerAndAdmin),
	row(PermTokensManage, "errors.permission.tokens_manage", ownerAndAdmin),
	row(PermIntegrationsManage, "errors.permission.integrations_manage", ownerAndAdmin),
	row(PermWebhooksManage, "errors.permission.webhooks_manage", ownerAndAdmin),
	row(PermSendingDomainsManage, "errors.permission.sending_domains_manage", ownerAndAdmin),
	row(PermContactsErase, "errors.permission.contacts_erase", ownerAndAdmin),
	row(PermContactsExport, "errors.permission.contacts_export", ownerAndAdmin),
	row(PermSuppressionsManage, "errors.permission.suppressions_manage", ownerAndAdmin),
	row(PermSecondFactorManage, "errors.permission.second_factor_manage", ownerAndAdmin),
	row(PermWorkspaceUpdate, "errors.permission.workspace_update", ownerAndAdmin),
	row(PermWorkspaceTimezone, "errors.permission.workspace_timezone", ownerAndAdmin),
	row(PermRevenueConfigure, "errors.permission.revenue_configure", ownerAndAdmin),
	row(PermBrandKitManage, "errors.permission.brand_kit_manage", ownerAndAdmin),
	row(PermJoinDomainsManage, "errors.permission.join_domains_manage", ownerAndAdmin),
	row(PermAccountLinkOpen, "errors.permission.account_link_open", ownerOnly),
	row(PermWorkspaceDelete, "errors.permission.workspace_delete", ownerOnly),
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

// errPermissionUnknown refuses a Permission outside the table, which no Role holds.
var errPermissionUnknown = validation.Forbidden("accounts: the permission is unknown", "errors.permission.unknown")

// grantOf is the row of the Permission p, false when the table has none.
func grantOf(p Permission) (grant, bool) {
	return lo.Find(grants, func(g grant) bool { return g.permission == p })
}

// Permissions is every Permission role holds, in the order of the access table.
func Permissions(role membership.Role) []Permission {
	held := lo.Filter(grants, func(g grant, _ int) bool { return Allows(role, g.permission) })
	return lo.Map(held, func(g grant, _ int) Permission { return g.permission })
}
