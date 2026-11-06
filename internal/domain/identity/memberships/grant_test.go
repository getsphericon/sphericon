package memberships_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
)

func TestAMemberGrantsTheScopesNoPermissionGuards(t *testing.T) {
	err := memberships.MayGrant(membership.RoleMember, []string{"contacts:read", "broadcasts:send"})

	assert.NoError(t, err)
}

func TestAMemberCannotGrantAScopeItsRoleLacksThePermissionFor(t *testing.T) {
	err := memberships.MayGrant(membership.RoleMember, []string{"contacts:read", "webhooks:write"})

	assert.ErrorIs(t, err, memberships.Require(membership.RoleMember, memberships.PermWebhooksManage))
}

func TestAnAdminGrantsTheScopesItsPermissionsGuard(t *testing.T) {
	err := memberships.MayGrant(membership.RoleAdmin, []string{"webhooks:write", "integrations:write", "contacts:export"})

	assert.NoError(t, err)
}

func TestTheFirstScopeBeyondTheRoleNamesTheRefusal(t *testing.T) {
	err := memberships.MayGrant(membership.RoleMember, []string{"integrations:write", "webhooks:write"})

	assert.ErrorIs(t, err, memberships.Require(membership.RoleMember, memberships.PermIntegrationsManage))
}

func TestGrantRequiresNamesOnlyTheGuardedScopes(t *testing.T) {
	needed := memberships.GrantRequires([]string{"contacts:read", "webhooks:read", "sending_domains:write"})

	assert.Equal(t, map[string]memberships.Permission{
		"webhooks:read":         memberships.PermWebhooksManage,
		"sending_domains:write": memberships.PermSendingDomainsManage,
	}, needed)
}
