package memberships_test

import (
	"errors"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/membership"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// cell is one cell of the Permission table of ADR 0061: whether role holds permission.
type cell struct {
	permission memberships.Permission
	role       membership.Role
	allowed    bool
}

// matrix is every cell of the Permission table: each Permission for each Role.
var matrix = []cell{
	{memberships.PermMembersManage, membership.RoleOwner, true},
	{memberships.PermMembersManage, membership.RoleAdmin, true},
	{memberships.PermMembersManage, membership.RoleMember, false},
	{memberships.PermAuditRead, membership.RoleOwner, true},
	{memberships.PermAuditRead, membership.RoleAdmin, true},
	{memberships.PermAuditRead, membership.RoleMember, false},
	{memberships.PermAuditConfigure, membership.RoleOwner, true},
	{memberships.PermAuditConfigure, membership.RoleAdmin, true},
	{memberships.PermAuditConfigure, membership.RoleMember, false},
	{memberships.PermTokensManage, membership.RoleOwner, true},
	{memberships.PermTokensManage, membership.RoleAdmin, true},
	{memberships.PermTokensManage, membership.RoleMember, false},
	{memberships.PermIntegrationsManage, membership.RoleOwner, true},
	{memberships.PermIntegrationsManage, membership.RoleAdmin, true},
	{memberships.PermIntegrationsManage, membership.RoleMember, false},
	{memberships.PermWebhooksManage, membership.RoleOwner, true},
	{memberships.PermWebhooksManage, membership.RoleAdmin, true},
	{memberships.PermWebhooksManage, membership.RoleMember, false},
	{memberships.PermSendingDomainsManage, membership.RoleOwner, true},
	{memberships.PermSendingDomainsManage, membership.RoleAdmin, true},
	{memberships.PermSendingDomainsManage, membership.RoleMember, false},
	{memberships.PermContactsErase, membership.RoleOwner, true},
	{memberships.PermContactsErase, membership.RoleAdmin, true},
	{memberships.PermContactsErase, membership.RoleMember, false},
	{memberships.PermContactsExport, membership.RoleOwner, true},
	{memberships.PermContactsExport, membership.RoleAdmin, true},
	{memberships.PermContactsExport, membership.RoleMember, false},
	{memberships.PermCustomFieldsManage, membership.RoleOwner, true},
	{memberships.PermCustomFieldsManage, membership.RoleAdmin, true},
	{memberships.PermCustomFieldsManage, membership.RoleMember, false},
	{memberships.PermSuppressionsManage, membership.RoleOwner, true},
	{memberships.PermSuppressionsManage, membership.RoleAdmin, true},
	{memberships.PermSuppressionsManage, membership.RoleMember, false},
	{memberships.PermSecondFactorManage, membership.RoleOwner, true},
	{memberships.PermSecondFactorManage, membership.RoleAdmin, true},
	{memberships.PermSecondFactorManage, membership.RoleMember, false},
	{memberships.PermWorkspaceUpdate, membership.RoleOwner, true},
	{memberships.PermWorkspaceUpdate, membership.RoleAdmin, true},
	{memberships.PermWorkspaceUpdate, membership.RoleMember, false},
	{memberships.PermWorkspaceTimezone, membership.RoleOwner, true},
	{memberships.PermWorkspaceTimezone, membership.RoleAdmin, true},
	{memberships.PermWorkspaceTimezone, membership.RoleMember, false},
	{memberships.PermRevenueConfigure, membership.RoleOwner, true},
	{memberships.PermRevenueConfigure, membership.RoleAdmin, true},
	{memberships.PermRevenueConfigure, membership.RoleMember, false},
	{memberships.PermBrandKitManage, membership.RoleOwner, true},
	{memberships.PermBrandKitManage, membership.RoleAdmin, true},
	{memberships.PermBrandKitManage, membership.RoleMember, false},
	{memberships.PermJoinDomainsManage, membership.RoleOwner, true},
	{memberships.PermJoinDomainsManage, membership.RoleAdmin, true},
	{memberships.PermJoinDomainsManage, membership.RoleMember, false},
	{memberships.PermTrackingDomainsManage, membership.RoleOwner, true},
	{memberships.PermTrackingDomainsManage, membership.RoleAdmin, true},
	{memberships.PermTrackingDomainsManage, membership.RoleMember, false},
	{memberships.PermHoldsResolve, membership.RoleOwner, true},
	{memberships.PermHoldsResolve, membership.RoleAdmin, true},
	{memberships.PermHoldsResolve, membership.RoleMember, false},
	{memberships.PermAccountLinkOpen, membership.RoleOwner, true},
	{memberships.PermAccountLinkOpen, membership.RoleAdmin, false},
	{memberships.PermAccountLinkOpen, membership.RoleMember, false},
	{memberships.PermWorkspaceDelete, membership.RoleOwner, true},
	{memberships.PermWorkspaceDelete, membership.RoleAdmin, false},
	{memberships.PermWorkspaceDelete, membership.RoleMember, false},
	{memberships.PermOwnersManage, membership.RoleOwner, true},
	{memberships.PermOwnersManage, membership.RoleAdmin, false},
	{memberships.PermOwnersManage, membership.RoleMember, false},
	{memberships.PermReleaseRead, membership.RoleOwner, true},
	{memberships.PermReleaseRead, membership.RoleAdmin, false},
	{memberships.PermReleaseRead, membership.RoleMember, false},
}

func TestAllowsFollowsEveryCellOfThePermissionMatrix(t *testing.T) {
	for _, c := range matrix {
		assert.Equal(t, c.allowed, memberships.Allows(c.role, c.permission), "%s for %s", c.permission, c.role)
	}
}

func TestRequireRefusesEveryCellAllowsDenies(t *testing.T) {
	for _, c := range matrix {
		err := memberships.Require(c.role, c.permission)
		refusal, refused := errors.AsType[*validation.RefusalError](err)
		assert.Equal(t, c.allowed, err == nil, "%s for %s", c.permission, c.role)
		assert.Equal(t, !c.allowed, refused && refusal.Kind() == validation.KindForbidden, "%s for %s", c.permission, c.role)
	}
}

// Refusal is the refusal a Role without the Permission gets, for an operation that found no
// Membership holding it at all.
func TestRefusalIsWhatEveryRoleWithoutThePermissionIsRefused(t *testing.T) {
	denied := lo.Filter(matrix, func(c cell, _ int) bool { return !c.allowed })
	for _, c := range denied {
		assert.Equal(t, memberships.Require(c.role, c.permission), memberships.Refusal(c.permission), "%s for %s", c.permission, c.role)
	}
}

func TestTheMatrixCoversEveryPermissionOfTheContract(t *testing.T) {
	inMatrix := lo.Uniq(lo.Map(matrix, func(c cell, _ int) string { return string(c.permission) }))
	contract := lo.Map(siteapi.Permission("").AllValues(), func(p siteapi.Permission, _ int) string { return string(p) })
	require.ElementsMatch(t, contract, inMatrix)
}

func TestPermissionsListsWhatARoleHoldsInTableOrder(t *testing.T) {
	all := lo.Map(siteapi.Permission("").AllValues(), func(p siteapi.Permission, _ int) memberships.Permission { return memberships.Permission(p) })
	assert.Equal(t, all, memberships.Permissions(membership.RoleOwner))
	assert.Equal(t, lo.Without(all, memberships.PermAccountLinkOpen, memberships.PermWorkspaceDelete, memberships.PermOwnersManage, memberships.PermReleaseRead), memberships.Permissions(membership.RoleAdmin))
	assert.Empty(t, memberships.Permissions(membership.RoleMember))
}
