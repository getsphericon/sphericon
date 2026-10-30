package accounts_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/internal/accounts"
)

func TestAMemberGrantsTheScopesNoPermissionGuards(t *testing.T) {
	err := accounts.MayGrant(membership.RoleMember, []string{"contacts:read", "broadcasts:send"})

	assert.NoError(t, err)
}

func TestAMemberCannotGrantAScopeItsRoleLacksThePermissionFor(t *testing.T) {
	err := accounts.MayGrant(membership.RoleMember, []string{"contacts:read", "webhooks:write"})

	assert.ErrorIs(t, err, accounts.Require(membership.RoleMember, accounts.PermWebhooksManage))
}

func TestAnAdminGrantsTheScopesItsPermissionsGuard(t *testing.T) {
	err := accounts.MayGrant(membership.RoleAdmin, []string{"webhooks:write", "integrations:write", "contacts:export"})

	assert.NoError(t, err)
}

func TestTheFirstScopeBeyondTheRoleNamesTheRefusal(t *testing.T) {
	err := accounts.MayGrant(membership.RoleMember, []string{"integrations:write", "webhooks:write"})

	assert.ErrorIs(t, err, accounts.Require(membership.RoleMember, accounts.PermIntegrationsManage))
}

func TestGrantRequiresNamesOnlyTheGuardedScopes(t *testing.T) {
	needed := accounts.GrantRequires([]string{"contacts:read", "webhooks:read", "sending_domains:write"})

	assert.Equal(t, map[string]accounts.Permission{
		"webhooks:read":         accounts.PermWebhooksManage,
		"sending_domains:write": accounts.PermSendingDomainsManage,
	}, needed)
}
