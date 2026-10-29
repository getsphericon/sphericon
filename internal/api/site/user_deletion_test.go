package site_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/recoverycode"
	entuser "github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/ent/useridentity"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func deleteUser(t *testing.T, c *siteapi.Client, in *siteapi.SiteDeleteUserInput) siteapi.SiteUserDeleteUserRes {
	t.Helper()
	res, err := c.SiteUserDeleteUser(t.Context(), in)
	require.NoError(t, err)
	return res
}

func userExists(t *testing.T, env *testhelper.TestEnv, email string) bool {
	t.Helper()
	exists, err := env.DB.User.Query().Where(entuser.Email(email)).Exist(t.Context())
	require.NoError(t, err)
	return exists
}

func TestAUserWithAPasswordDeletesThemself(t *testing.T) {
	env := testhelper.Setup(t)
	session := env.SiteToken(t, fixtures.MemberMaryEmail, nil)
	mary := env.SiteActor(t, fixtures.MemberMaryEmail)

	res := deleteUser(t, mary, &siteapi.SiteDeleteUserInput{
		Email:           fixtures.MemberMaryEmail,
		CurrentPassword: siteapi.NewOptString(fixtures.MemberMaryPassword),
	})
	assert.IsType(t, &siteapi.SiteUserDeleteUserNoContent{}, res)

	assert.False(t, userExists(t, env, fixtures.MemberMaryEmail))
	old := sendWithSession(t, env, http.MethodGet, "/site/me", session, "")
	assert.Equal(t, http.StatusUnauthorized, old.Code, "a deleted User's session is dead")
	mail := env.SystemMail.Messages()
	require.Len(t, mail, 1)
	assert.Equal(t, fixtures.MemberMaryEmail, mail[0].To)
	assert.Contains(t, mail[0].Subject, "deleted")
}

func TestAWrongPasswordDeletesNothingAndCountsOnTheLoginBudget(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithRateLimits(config.RateLimits{LoginFailures: 3}))
	mary := env.SiteActor(t, fixtures.MemberMaryEmail)

	for range 3 {
		res := deleteUser(t, mary, &siteapi.SiteDeleteUserInput{Email: fixtures.MemberMaryEmail, CurrentPassword: siteapi.NewOptString("wrong")})
		require.IsType(t, &siteapi.SiteUserDeleteUserForbidden{}, res)
	}

	assert.True(t, userExists(t, env, fixtures.MemberMaryEmail))
	assert.Equal(t, http.StatusTooManyRequests, loginStatus(t, env, fixtures.MemberMaryEmail, fixtures.MemberMaryPassword))
	right := deleteUser(t, mary, &siteapi.SiteDeleteUserInput{Email: fixtures.MemberMaryEmail, CurrentPassword: siteapi.NewOptString(fixtures.MemberMaryPassword)})
	assert.IsType(t, &siteapi.ProblemDetailsHeaders{}, right, "while the delay runs even the right password is refused")
	assert.True(t, userExists(t, env, fixtures.MemberMaryEmail))
}

func TestATypedEmailThatIsNotTheirsIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	mary := env.SiteActor(t, fixtures.MemberMaryEmail)

	res := deleteUser(t, mary, &siteapi.SiteDeleteUserInput{Email: fixtures.OwnerJohnEmail, CurrentPassword: siteapi.NewOptString(fixtures.MemberMaryPassword)})

	refused, ok := res.(*siteapi.SiteDeleteUserProblemDetails)
	require.Truef(t, ok, "got %T", res)
	assert.Contains(t, refused.Errors.Value["email"][0], "own email")
	assert.Empty(t, refused.Workspaces)
	assert.True(t, userExists(t, env, fixtures.MemberMaryEmail))
}

func totpCode(t *testing.T) string {
	t.Helper()
	code, err := totp.GenerateCode(fixtures.SecondFactorSamTotpSecret, time.Now())
	require.NoError(t, err)
	return code
}

func TestAUserWithASecondFactorConfirmsWithTheirCodeAndItsRowsGo(t *testing.T) {
	env := testhelper.Setup(t)
	sam := env.SiteActor(t, fixtures.SecondFactorSamEmail)

	res := deleteUser(t, sam, &siteapi.SiteDeleteUserInput{
		Email:           fixtures.SecondFactorSamEmail,
		CurrentPassword: siteapi.NewOptString(fixtures.SecondFactorSamPassword),
		Code:            siteapi.NewOptString(totpCode(t)),
	})
	require.IsType(t, &siteapi.SiteUserDeleteUserNoContent{}, res)

	assert.False(t, userExists(t, env, fixtures.SecondFactorSamEmail))
	codes, err := env.DB.RecoveryCode.Query().Where(recoverycode.UserID(fixtures.SecondFactorSamID)).Count(t.Context())
	require.NoError(t, err)
	assert.Zero(t, codes)
	identities, err := env.DB.UserIdentity.Query().Where(useridentity.UserID(fixtures.SecondFactorSamID)).Count(t.Context())
	require.NoError(t, err)
	assert.Zero(t, identities)
	memberships, err := env.DB.Membership.Query().Where(membership.UserID(fixtures.SecondFactorSamID)).Count(t.Context())
	require.NoError(t, err)
	assert.Zero(t, memberships)
}

func TestAUserWithASecondFactorCanConfirmWithARecoveryCode(t *testing.T) {
	env := testhelper.Setup(t)
	sam := env.SiteActor(t, fixtures.SecondFactorSamEmail)

	res := deleteUser(t, sam, &siteapi.SiteDeleteUserInput{
		Email:           fixtures.SecondFactorSamEmail,
		CurrentPassword: siteapi.NewOptString(fixtures.SecondFactorSamPassword),
		Code:            siteapi.NewOptString(fixtures.SecondFactorSamRecoveryCode),
	})

	assert.IsType(t, &siteapi.SiteUserDeleteUserNoContent{}, res)
}

func TestAMissingOrWrongCodeDeletesNothingAndCountsOnTheLoginBudget(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithRateLimits(config.RateLimits{LoginFailures: 3}))
	sam := env.SiteActor(t, fixtures.SecondFactorSamEmail)
	input := func(code siteapi.OptString) *siteapi.SiteDeleteUserInput {
		return &siteapi.SiteDeleteUserInput{Email: fixtures.SecondFactorSamEmail, CurrentPassword: siteapi.NewOptString(fixtures.SecondFactorSamPassword), Code: code}
	}

	missing := deleteUser(t, sam, input(siteapi.OptString{}))
	require.IsType(t, &siteapi.SiteUserDeleteUserForbidden{}, missing)
	for range 2 {
		wrong := deleteUser(t, sam, input(siteapi.NewOptString("000000")))
		require.IsType(t, &siteapi.SiteUserDeleteUserForbidden{}, wrong)
	}

	assert.True(t, userExists(t, env, fixtures.SecondFactorSamEmail))
	right := deleteUser(t, sam, input(siteapi.NewOptString(totpCode(t))))
	assert.IsType(t, &siteapi.ProblemDetailsHeaders{}, right, "three wrong codes took the delay, even the right code waits")
	assert.True(t, userExists(t, env, fixtures.SecondFactorSamEmail))
}

func TestTheOnlyOwnerOfLiveWorkspacesIsRefusedWithTheirList(t *testing.T) {
	env := testhelper.Setup(t)
	dora := env.SiteActor(t, fixtures.SoleOwnerDoraEmail)

	res := deleteUser(t, dora, &siteapi.SiteDeleteUserInput{Email: fixtures.SoleOwnerDoraEmail, CurrentPassword: siteapi.NewOptString(fixtures.SoleOwnerDoraPassword)})

	refused, ok := res.(*siteapi.SiteDeleteUserProblemDetails)
	require.Truef(t, ok, "got %T", res)
	listed := lo.Map(refused.Workspaces, func(w siteapi.SiteSoleOwnedWorkspace, _ int) string { return w.Name })
	assert.Equal(t, []string{fixtures.PiedPiperName, fixtures.AviatoName}, listed)
	assert.Equal(t, siteapi.WorkspaceId(fixtures.PiedPiperPublicID), refused.Workspaces[0].ID)
	assert.True(t, userExists(t, env, fixtures.SoleOwnerDoraEmail))
	assert.Empty(t, env.SystemMail.Messages())
	assert.Equal(t, []siteapi.WorkspaceId{fixtures.PiedPiperPublicID, fixtures.AviatoPublicID}, workspaceIDs(t, dora))
}

func TestTheOnlyOwnerOfAWorkspaceInItsDeletionWindowIsNotBlocked(t *testing.T) {
	env := testhelper.Setup(t)
	dan := env.SiteActor(t, fixtures.DeletedWorkspaceOwnerDanEmail)

	res := deleteUser(t, dan, &siteapi.SiteDeleteUserInput{
		Email: fixtures.DeletedWorkspaceOwnerDanEmail, CurrentPassword: siteapi.NewOptString(fixtures.DeletedWorkspaceOwnerDanPassword),
	})

	require.IsType(t, &siteapi.SiteUserDeleteUserNoContent{}, res)
	assert.False(t, userExists(t, env, fixtures.DeletedWorkspaceOwnerDanEmail))
	left, err := env.DB.Membership.Query().Where(membership.WorkspaceID(fixtures.BluthID)).Count(t.Context())
	require.NoError(t, err)
	assert.Zero(t, left)
}

func TestADeletedAddressSignsUpAgainWhereSignupIsOpen(t *testing.T) {
	env := testhelper.Setup(t)
	deleteUser(t, env.SiteActor(t, fixtures.MemberMaryEmail), &siteapi.SiteDeleteUserInput{
		Email: fixtures.MemberMaryEmail, CurrentPassword: siteapi.NewOptString(fixtures.MemberMaryPassword),
	})

	res := signUpWithPassword(t, env, "Mary Again", fixtures.MemberMaryEmail, formPassword)

	require.IsType(t, &siteapi.SiteAuthSignUpAccepted{}, res)
	again, err := env.DB.User.Query().Where(entuser.Email(fixtures.MemberMaryEmail)).Only(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "Mary Again", again.Name)
	assert.NotEqual(t, int64(fixtures.MemberMaryID), again.ID, "a new User, not the old one")
}

func TestADeletedAddressCannotSignUpWhereSignupIsClosed(t *testing.T) {
	env := closedInstance(t)
	deleteUser(t, env.SiteActor(t, fixtures.MemberMaryEmail), &siteapi.SiteDeleteUserInput{
		Email: fixtures.MemberMaryEmail, CurrentPassword: siteapi.NewOptString(fixtures.MemberMaryPassword),
	})

	res := signUpWithPassword(t, env, "Mary Again", fixtures.MemberMaryEmail, formPassword)

	assert.IsType(t, &siteapi.SiteAuthSignUpForbidden{}, res)
	assert.False(t, userExists(t, env, fixtures.MemberMaryEmail))
}

func TestAnInvitationSentByTheDeletedUserStaysValidWithoutASenderAndCanBeRevoked(t *testing.T) {
	env := testhelper.Setup(t)
	deleteUser(t, env.SiteActor(t, fixtures.AdminAliceEmail), &siteapi.SiteDeleteUserInput{
		Email: fixtures.AdminAliceEmail, CurrentPassword: siteapi.NewOptString(fixtures.AdminAlicePassword),
	})
	john := env.SiteActor(t, fixtures.OwnerJohnEmail)

	list, err := john.SiteInvitationsList(t.Context(), siteapi.SiteInvitationsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	items, ok := list.(*siteapi.SiteInvitationsListOKApplicationJSON)
	require.Truef(t, ok, "got %T", list)
	sent, found := lo.Find(*items, func(inv siteapi.SiteInvitationResource) bool {
		return string(inv.Email) == fixtures.InvitationSentByAliceEmail
	})
	require.True(t, found)
	assert.True(t, sent.InvitedByEmail.Null || !sent.InvitedByEmail.Set, "no sender is shown")

	revoked, err := john.SiteInvitationsDelete(t.Context(), siteapi.SiteInvitationsDeleteParams{
		WorkspaceId: fixtures.AcmePublicID, ID: sent.ID,
	})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteInvitationsDeleteNoContent{}, revoked)
}

func TestAContactAndAStaffMemberWithTheUsersAddressAreUntouched(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	// John is no longer Acme's only Owner, so his deletion goes through.
	require.NoError(t, acme.Membership().UpdateOneID(fixtures.AcmeMemberMembershipID).SetRole(membership.RoleOwner).Exec(t.Context()))
	// A Contact that carries the User's address (a fixture Contact moves to it).
	contact, err := acme.Contact().UpdateOneID(fixtures.ContactAliceID).SetEmail(fixtures.OwnerJohnEmail).SetFirstName("Johnny").Save(t.Context())
	require.NoError(t, err)
	staffBefore := env.DB.StaffMember.GetX(t.Context(), fixtures.StaffAlsoCustomerID)

	res := deleteUser(t, env.SiteActor(t, fixtures.OwnerJohnEmail), &siteapi.SiteDeleteUserInput{
		Email: fixtures.OwnerJohnEmail, CurrentPassword: siteapi.NewOptString(fixtures.OwnerJohnPassword),
	})
	require.IsType(t, &siteapi.SiteUserDeleteUserNoContent{}, res)

	contactAfter, err := acme.Contact().Get(t.Context(), contact.ID)
	require.NoError(t, err)
	assert.Equal(t, new("Johnny"), contactAfter.FirstName)
	staffAfter := env.DB.StaffMember.GetX(t.Context(), fixtures.StaffAlsoCustomerID)
	assert.Equal(t, staffBefore.Email, staffAfter.Email)
	assert.Equal(t, staffBefore.PasswordHash, staffAfter.PasswordHash)
	assert.Equal(t, staffBefore.TotpSecretEncrypted, staffAfter.TotpSecretEncrypted)
}

func TestAuditEntriesOfTheDeletedUserKeepTheirIdAndName(t *testing.T) {
	env := testhelper.Setup(t)
	deleteUser(t, env.SiteActor(t, fixtures.MemberMaryEmail), &siteapi.SiteDeleteUserInput{
		Email: fixtures.MemberMaryEmail, CurrentPassword: siteapi.NewOptString(fixtures.MemberMaryPassword),
	})

	got := entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, events.ActionMembershipDelete)

	require.Len(t, got, 1)
	assert.Equal(t, siteapi.SiteAuditActorKindUser, got[0].Actor.Kind)
	assert.Equal(t, fixtures.MemberMaryName, got[0].Actor.Name.Value)
	assert.Equal(t, "3", got[0].Actor.ID.Value)
	globex := entriesNamed(t, env, fixtures.OwnerJaneEmail, fixtures.GlobexPublicID, events.ActionMembershipDelete)
	assert.Len(t, globex, 1, "every Workspace the User belonged to records its own removal")
}

func TestDeletingAUserRemovesTheirPasskeys(t *testing.T) {
	env := testhelper.Setup(t)
	pat := env.SiteActor(t, fixtures.PasskeyPatEmail)
	require.Equal(t, 1, passkeysOf(t, env, fixtures.PasskeyPatID))

	res := deleteUser(t, pat, &siteapi.SiteDeleteUserInput{
		Email:           fixtures.PasskeyPatEmail,
		CurrentPassword: siteapi.NewOptString(fixtures.PasskeyPatPassword),
	})

	assert.IsType(t, &siteapi.SiteUserDeleteUserNoContent{}, res)
	assert.False(t, userExists(t, env, fixtures.PasskeyPatEmail))
	assert.Zero(t, passkeysOf(t, env, fixtures.PasskeyPatID))
}
