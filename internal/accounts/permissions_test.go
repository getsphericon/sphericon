package accounts_test

import (
	"errors"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/membership"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/validation"
)

// cell is one cell of the Permission table of ADR 0061: whether role holds permission.
type cell struct {
	permission accounts.Permission
	role       membership.Role
	allowed    bool
}

// matrix is every cell of the Permission table: each Permission for each Role.
var matrix = []cell{
	{accounts.PermMembersManage, membership.RoleOwner, true},
	{accounts.PermMembersManage, membership.RoleAdmin, true},
	{accounts.PermMembersManage, membership.RoleMember, false},
	{accounts.PermAuditRead, membership.RoleOwner, true},
	{accounts.PermAuditRead, membership.RoleAdmin, true},
	{accounts.PermAuditRead, membership.RoleMember, false},
	{accounts.PermAuditConfigure, membership.RoleOwner, true},
	{accounts.PermAuditConfigure, membership.RoleAdmin, true},
	{accounts.PermAuditConfigure, membership.RoleMember, false},
	{accounts.PermTokensManage, membership.RoleOwner, true},
	{accounts.PermTokensManage, membership.RoleAdmin, true},
	{accounts.PermTokensManage, membership.RoleMember, false},
	{accounts.PermIntegrationsManage, membership.RoleOwner, true},
	{accounts.PermIntegrationsManage, membership.RoleAdmin, true},
	{accounts.PermIntegrationsManage, membership.RoleMember, false},
	{accounts.PermWebhooksManage, membership.RoleOwner, true},
	{accounts.PermWebhooksManage, membership.RoleAdmin, true},
	{accounts.PermWebhooksManage, membership.RoleMember, false},
	{accounts.PermSendingDomainsManage, membership.RoleOwner, true},
	{accounts.PermSendingDomainsManage, membership.RoleAdmin, true},
	{accounts.PermSendingDomainsManage, membership.RoleMember, false},
	{accounts.PermContactsErase, membership.RoleOwner, true},
	{accounts.PermContactsErase, membership.RoleAdmin, true},
	{accounts.PermContactsErase, membership.RoleMember, false},
	{accounts.PermContactsExport, membership.RoleOwner, true},
	{accounts.PermContactsExport, membership.RoleAdmin, true},
	{accounts.PermContactsExport, membership.RoleMember, false},
	{accounts.PermSuppressionsManage, membership.RoleOwner, true},
	{accounts.PermSuppressionsManage, membership.RoleAdmin, true},
	{accounts.PermSuppressionsManage, membership.RoleMember, false},
	{accounts.PermSecondFactorManage, membership.RoleOwner, true},
	{accounts.PermSecondFactorManage, membership.RoleAdmin, true},
	{accounts.PermSecondFactorManage, membership.RoleMember, false},
	{accounts.PermWorkspaceUpdate, membership.RoleOwner, true},
	{accounts.PermWorkspaceUpdate, membership.RoleAdmin, true},
	{accounts.PermWorkspaceUpdate, membership.RoleMember, false},
	{accounts.PermWorkspaceTimezone, membership.RoleOwner, true},
	{accounts.PermWorkspaceTimezone, membership.RoleAdmin, true},
	{accounts.PermWorkspaceTimezone, membership.RoleMember, false},
	{accounts.PermRevenueConfigure, membership.RoleOwner, true},
	{accounts.PermRevenueConfigure, membership.RoleAdmin, true},
	{accounts.PermRevenueConfigure, membership.RoleMember, false},
	{accounts.PermBrandKitManage, membership.RoleOwner, true},
	{accounts.PermBrandKitManage, membership.RoleAdmin, true},
	{accounts.PermBrandKitManage, membership.RoleMember, false},
	{accounts.PermJoinDomainsManage, membership.RoleOwner, true},
	{accounts.PermJoinDomainsManage, membership.RoleAdmin, true},
	{accounts.PermJoinDomainsManage, membership.RoleMember, false},
	{accounts.PermTrackingDomainsManage, membership.RoleOwner, true},
	{accounts.PermTrackingDomainsManage, membership.RoleAdmin, true},
	{accounts.PermTrackingDomainsManage, membership.RoleMember, false},
	{accounts.PermHoldsResolve, membership.RoleOwner, true},
	{accounts.PermHoldsResolve, membership.RoleAdmin, true},
	{accounts.PermHoldsResolve, membership.RoleMember, false},
	{accounts.PermAccountLinkOpen, membership.RoleOwner, true},
	{accounts.PermAccountLinkOpen, membership.RoleAdmin, false},
	{accounts.PermAccountLinkOpen, membership.RoleMember, false},
	{accounts.PermWorkspaceDelete, membership.RoleOwner, true},
	{accounts.PermWorkspaceDelete, membership.RoleAdmin, false},
	{accounts.PermWorkspaceDelete, membership.RoleMember, false},
	{accounts.PermOwnersManage, membership.RoleOwner, true},
	{accounts.PermOwnersManage, membership.RoleAdmin, false},
	{accounts.PermOwnersManage, membership.RoleMember, false},
	{accounts.PermReleaseRead, membership.RoleOwner, true},
	{accounts.PermReleaseRead, membership.RoleAdmin, false},
	{accounts.PermReleaseRead, membership.RoleMember, false},
}

func TestAllowsFollowsEveryCellOfThePermissionMatrix(t *testing.T) {
	for _, c := range matrix {
		assert.Equal(t, c.allowed, accounts.Allows(c.role, c.permission), "%s for %s", c.permission, c.role)
	}
}

func TestRequireRefusesEveryCellAllowsDenies(t *testing.T) {
	for _, c := range matrix {
		err := accounts.Require(c.role, c.permission)
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
		assert.Equal(t, accounts.Require(c.role, c.permission), accounts.Refusal(c.permission), "%s for %s", c.permission, c.role)
	}
}

func TestTheMatrixCoversEveryPermissionOfTheContract(t *testing.T) {
	inMatrix := lo.Uniq(lo.Map(matrix, func(c cell, _ int) string { return string(c.permission) }))
	contract := lo.Map(siteapi.Permission("").AllValues(), func(p siteapi.Permission, _ int) string { return string(p) })
	require.ElementsMatch(t, contract, inMatrix)
}

func TestPermissionsListsWhatARoleHoldsInTableOrder(t *testing.T) {
	all := lo.Map(siteapi.Permission("").AllValues(), func(p siteapi.Permission, _ int) accounts.Permission { return accounts.Permission(p) })
	assert.Equal(t, all, accounts.Permissions(membership.RoleOwner))
	assert.Equal(t, lo.Without(all, accounts.PermAccountLinkOpen, accounts.PermWorkspaceDelete, accounts.PermOwnersManage, accounts.PermReleaseRead), accounts.Permissions(membership.RoleAdmin))
	assert.Empty(t, accounts.Permissions(membership.RoleMember))
}
