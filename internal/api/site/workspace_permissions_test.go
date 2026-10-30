package site_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The Workspace of the current User carries the Permissions of their Role (ADR 0061), so the
// client reads them instead of computing them from the Role.
func TestTheWorkspaceCarriesThePermissionsOfTheCallersRole(t *testing.T) {
	env := testhelper.Setup(t)
	all := siteapi.Permission("").AllValues()

	owner := workspaceOf(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.AcmePublicID)
	admin := workspaceOf(t, env.SiteActor(t, fixtures.AdminAliceEmail), fixtures.AcmePublicID)
	member := workspaceOf(t, env.SiteActor(t, fixtures.MemberMaryEmail), fixtures.AcmePublicID)

	assert.Equal(t, all, owner.Permissions)
	assert.Equal(t, lo.Without(all, siteapi.PermissionAccountLinkOpen, siteapi.PermissionWorkspaceDelete, siteapi.PermissionOwnersManage, siteapi.PermissionReleaseRead), admin.Permissions)
	assert.Empty(t, member.Permissions)
}
