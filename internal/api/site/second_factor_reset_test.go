package site_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/passkey"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func resetSecondFactor(t *testing.T, env *testhelper.TestEnv, email string, workspaceID siteapi.WorkspaceId, membershipID int64) siteapi.SiteMembershipsResetSecondFactorRes {
	t.Helper()
	res, err := env.SiteActor(t, email).SiteMembershipsResetSecondFactor(t.Context(),
		siteapi.SiteMembershipsResetSecondFactorParams{WorkspaceId: workspaceID, ID: idStr(membershipID)})
	require.NoError(t, err)
	return res
}

func TestOwnerResetsAMembersSecondFactorAndEndsTheirSessions(t *testing.T) {
	env := testhelper.Setup(t)
	sam := env.SiteToken(t, fixtures.SecondFactorSamEmail, nil)
	jane := env.SiteToken(t, fixtures.OwnerJaneEmail, nil)

	res := sendWithSession(t, env, http.MethodPost, fmt.Sprintf("/site/workspaces/%s/memberships/%d/reset-second-factor",
		fixtures.GlobexPublicID, fixtures.GlobexSamMembershipID), jane, "")
	require.Equal(t, http.StatusNoContent, res.Code)

	assert.Empty(t, reissued(t, res), "the acting session belongs to someone else: nothing to reissue")
	assert.Equal(t, http.StatusOK, workspacesStatus(t, env, jane), "the acting session continues")
	assert.Equal(t, http.StatusUnauthorized, workspacesStatus(t, env, sam), "the member's sessions end")
	st, err := env.SecondFactor.Status(t.Context(), fixtures.SecondFactorSamID)
	require.NoError(t, err)
	assert.False(t, st.Enabled)
	assert.Zero(t, st.RecoveryCodesRemaining)

	got := entriesNamed(t, env, fixtures.OwnerJaneEmail, fixtures.GlobexPublicID, events.ActionUserSecondFactorReset)
	require.Len(t, got, 1)
	assert.Equal(t, "Jane", got[0].Actor.Name.Value)
	assert.Equal(t, "user", got[0].Target.Type)
}

func TestAdminResetsAMembersSecondFactor(t *testing.T) {
	env := testhelper.Setup(t)
	require.NoError(t, env.DB.Membership.UpdateOneID(fixtures.GlobexMaryMembershipID).SetRole(membership.RoleAdmin).Exec(t.Context()))

	res := resetSecondFactor(t, env, fixtures.MemberMaryEmail, fixtures.GlobexPublicID, fixtures.GlobexSamMembershipID)

	require.IsType(t, &siteapi.SiteMembershipsResetSecondFactorNoContent{}, res)
}

func TestMemberCannotResetASecondFactor(t *testing.T) {
	env := testhelper.Setup(t)

	res := resetSecondFactor(t, env, fixtures.MemberMaryEmail, fixtures.GlobexPublicID, fixtures.GlobexSamMembershipID)

	require.IsType(t, &siteapi.SiteMembershipsResetSecondFactorForbidden{}, res)
	st, err := env.SecondFactor.Status(t.Context(), fixtures.SecondFactorSamID)
	require.NoError(t, err)
	assert.True(t, st.Enabled)
}

func TestSecondFactorResetNeedsAMembershipInTheWorkspace(t *testing.T) {
	env := testhelper.Setup(t)

	res := resetSecondFactor(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, fixtures.GlobexSamMembershipID)

	require.IsType(t, &siteapi.SiteMembershipsResetSecondFactorNotFound{}, res)
}

func TestAdminCannotResetAnOwnersSecondFactor(t *testing.T) {
	env := testhelper.Setup(t)
	require.NoError(t, env.DB.Membership.UpdateOneID(fixtures.GlobexMaryMembershipID).SetRole(membership.RoleAdmin).Exec(t.Context()))

	res := resetSecondFactor(t, env, fixtures.MemberMaryEmail, fixtures.GlobexPublicID, fixtures.GlobexOwnerMembershipID)

	require.IsType(t, &siteapi.SiteMembershipsResetSecondFactorForbidden{}, res)
	assert.Equal(t, siteapi.ProblemCodePermissionRequired, res.(*siteapi.SiteMembershipsResetSecondFactorForbidden).Code.Value)
}

func TestSecondFactorResetRefusesOneselfAndAMemberWithoutAFactor(t *testing.T) {
	env := testhelper.Setup(t)

	self := resetSecondFactor(t, env, fixtures.OwnerJaneEmail, fixtures.GlobexPublicID, fixtures.GlobexOwnerMembershipID)
	none := resetSecondFactor(t, env, fixtures.OwnerJaneEmail, fixtures.GlobexPublicID, fixtures.GlobexMaryMembershipID)

	require.IsType(t, &siteapi.SiteMembershipsResetSecondFactorUnprocessableEntity{}, self)
	require.IsType(t, &siteapi.SiteMembershipsResetSecondFactorUnprocessableEntity{}, none)
}

func passkeysOf(t *testing.T, env *testhelper.TestEnv, userID int64) int {
	t.Helper()
	n, err := env.DB.Passkey.Query().Where(passkey.UserID(userID)).Count(t.Context())
	require.NoError(t, err)
	return n
}

func TestResetRemovesAMembersPasskeysAndKeepsItsAuditEntry(t *testing.T) {
	env := testhelper.Setup(t)
	pat := patSession(t, env)
	require.Equal(t, 1, passkeysOf(t, env, fixtures.PasskeyPatID))

	res := resetSecondFactor(t, env, fixtures.OwnerJaneEmail, fixtures.GlobexPublicID, fixtures.GlobexPatMembershipID)

	require.IsType(t, &siteapi.SiteMembershipsResetSecondFactorNoContent{}, res)
	assert.Zero(t, passkeysOf(t, env, fixtures.PasskeyPatID))
	assert.Equal(t, http.StatusUnauthorized, workspacesStatus(t, env, pat), "the member's sessions end")
	got := entriesNamed(t, env, fixtures.OwnerJaneEmail, fixtures.GlobexPublicID, events.ActionUserSecondFactorReset)
	require.Len(t, got, 1)
	assert.Equal(t, "user", got[0].Target.Type)
}

func TestAMemberWhoseSecondFactorWasResetCannotLogInWithTheirOldPasskey(t *testing.T) {
	env := testhelper.Setup(t)
	options, ceremony := beginLogin(t, env)
	assertion := env.FixturePasskey(t).Assert(options)

	res := resetSecondFactor(t, env, fixtures.OwnerJaneEmail, fixtures.GlobexPublicID, fixtures.GlobexPatMembershipID)
	require.IsType(t, &siteapi.SiteMembershipsResetSecondFactorNoContent{}, res)

	rec := finishLogin(t, env, ceremony, assertion)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Nil(t, sessionCookie(rec.Result()))
}
