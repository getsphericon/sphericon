package site_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/passkey"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/identity/secondfactors"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func resetSecondFactor(t *testing.T, env *testhelper.TestEnv, email string, workspaceID siteapi.WorkspaceId, membershipID int64) error {
	t.Helper()
	return env.SiteActor(t, email).SiteMembershipsResetSecondFactor(t.Context(),
		siteapi.SiteMembershipsResetSecondFactorParams{WorkspaceId: workspaceID, ID: idStr(membershipID)})
}

func TestOwnerResetsAMembersSecondFactorAndEndsTheirSessions(t *testing.T) {
	env := testhelper.Setup(t)
	reese := env.SiteToken(t, fixtures.SecondFactorReeseEmail, nil)
	olga := env.SiteToken(t, fixtures.ResetOwnerOlgaEmail, nil)

	res := sendWithSession(t, env, http.MethodPost, fmt.Sprintf("/site/workspaces/%s/memberships/%s/reset-second-factor", fixtures.OllivandersPublicID, testhelper.ID(fixtures.OllivandersReeseMembershipID)), olga, "")
	require.Equal(t, http.StatusNoContent, res.Code)

	assert.Empty(t, reissued(t, res), "the acting session belongs to someone else: nothing to reissue")
	assert.Equal(t, http.StatusOK, workspacesStatus(t, env, olga), "the acting session continues")
	assert.Equal(t, http.StatusUnauthorized, workspacesStatus(t, env, reese), "the member's sessions end")
	st, err := testhelper.Module[*secondfactors.Module](env).Status(t.Context(), fixtures.SecondFactorReeseID)
	require.NoError(t, err)
	assert.False(t, st.Enabled)
	assert.Zero(t, st.RecoveryCodesRemaining)

	for _, ws := range []siteapi.WorkspaceId{fixtures.OllivandersPublicID, fixtures.GringottsPublicID} {
		got := entriesNamed(t, env, fixtures.ResetOwnerOlgaEmail, ws, events.ActionUserSecondFactorReset)
		require.Len(t, got, 1, "recorded in every Workspace of the member")
		assert.Equal(t, "Olga", got[0].Actor.Name.Value)
		assert.Equal(t, "user", got[0].Target.Type)
	}
	sent := env.SystemMail.Messages()
	require.Len(t, sent, 1)
	assert.Equal(t, fixtures.SecondFactorReeseEmail, sent[0].To)
}

func TestAdminResetsAMembersSecondFactor(t *testing.T) {
	env := testhelper.Setup(t)

	err := resetSecondFactor(t, env, fixtures.ResetAdminAbeEmail, fixtures.OllivandersPublicID, fixtures.OllivandersReeseMembershipID)

	require.NoError(t, err)
}

// A member who belongs to a Workspace where the caller may not reset their Second factor is
// refused on the member, in words that name no Workspace (ADR 0177).
func TestSecondFactorResetOfAMemberOfAnotherWorkspaceIsRefused(t *testing.T) {
	env := testhelper.Setup(t)

	err := resetSecondFactor(t, env, fixtures.OwnerJaneEmail, fixtures.GlobexPublicID, fixtures.GlobexSamMembershipID)

	problem := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	detail := problem.Errors.Value["member"]
	require.Len(t, detail, 1)
	assert.Contains(t, detail[0], "also belongs to other workspaces")
	assert.NotContains(t, detail[0], fixtures.UmbrellaName)
	st, err := testhelper.Module[*secondfactors.Module](env).Status(t.Context(), fixtures.SecondFactorSamID)
	require.NoError(t, err)
	assert.True(t, st.Enabled)
	assert.Empty(t, env.SystemMail.Messages())
}

func TestMemberCannotResetASecondFactor(t *testing.T) {
	env := testhelper.Setup(t)

	err := resetSecondFactor(t, env, fixtures.MemberMaryEmail, fixtures.GlobexPublicID, fixtures.GlobexSamMembershipID)

	testhelper.SiteRefusal(t, err, http.StatusForbidden)
	st, err := testhelper.Module[*secondfactors.Module](env).Status(t.Context(), fixtures.SecondFactorSamID)
	require.NoError(t, err)
	assert.True(t, st.Enabled)
}

func TestSecondFactorResetNeedsAMembershipInTheWorkspace(t *testing.T) {
	env := testhelper.Setup(t)

	err := resetSecondFactor(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, fixtures.GlobexSamMembershipID)

	testhelper.SiteRefusal(t, err, http.StatusNotFound)
}

func TestAdminCannotResetAnOwnersSecondFactor(t *testing.T) {
	env := testhelper.Setup(t)
	env.Feed(t, func(tx *ent.Client) error {
		return tx.Membership.UpdateOneID(fixtures.GlobexMaryMembershipID).SetRole(membership.RoleAdmin).Exec(t.Context())
	})

	err := resetSecondFactor(t, env, fixtures.MemberMaryEmail, fixtures.GlobexPublicID, fixtures.GlobexOwnerMembershipID)

	problem := testhelper.SiteRefusal(t, err, http.StatusForbidden)
	assert.Equal(t, siteapi.ProblemCodePermissionRequired, problem.Code.Value)
}

func TestSecondFactorResetRefusesOneselfAndAMemberWithoutAFactor(t *testing.T) {
	env := testhelper.Setup(t)

	self := resetSecondFactor(t, env, fixtures.OwnerJaneEmail, fixtures.GlobexPublicID, fixtures.GlobexOwnerMembershipID)
	none := resetSecondFactor(t, env, fixtures.ResetOwnerOlgaEmail, fixtures.OllivandersPublicID, fixtures.OllivandersAbeMembershipID)

	testhelper.SiteRefusal(t, self, http.StatusUnprocessableEntity)
	testhelper.SiteRefusal(t, none, http.StatusUnprocessableEntity)
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

	err := resetSecondFactor(t, env, fixtures.OwnerJaneEmail, fixtures.GlobexPublicID, fixtures.GlobexPatMembershipID)

	require.NoError(t, err)
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

	require.NoError(t, resetSecondFactor(t, env, fixtures.OwnerJaneEmail, fixtures.GlobexPublicID, fixtures.GlobexPatMembershipID))

	rec := finishLogin(t, env, ceremony, assertion)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Nil(t, sessionCookie(rec.Result()))
}
