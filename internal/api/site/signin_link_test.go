package site_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	entuser "github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/ent/useridentity"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const (
	patEmail    = "pat@gmail.com"
	patPassword = "pat-password-1"
)

// patWithGoogle is Pat's Google account: the address Pat registered with, vouched for by Google.
var patWithGoogle = testhelper.SigninIdentity{Subject: "google-subject-of-pat", Email: patEmail, EmailVerified: true, Name: "Pat at Google"}

// johnWithGoogle is a Google account of a hosted domain whose address is OwnerJohn's: an address
// the fixtures hold as Verified but never proved (no email_proven_at).
var johnWithGoogle = testhelper.SigninIdentity{
	Subject: "google-subject-of-a-john", Email: fixtures.OwnerJohnEmail, EmailVerified: true, HostedDomain: "getsphericon.com", Name: "John",
}

func identitiesOf(t *testing.T, env *testhelper.TestEnv, u *ent.User) []*ent.UserIdentity {
	t.Helper()
	identities, err := env.DB.UserIdentity.Query().Where(useridentity.UserID(u.ID)).All(t.Context())
	require.NoError(t, err)
	return identities
}

func usersWithEmail(t *testing.T, env *testhelper.TestEnv, email string) int {
	t.Helper()
	n, err := env.DB.User.Query().Where(entuser.EmailEqualFold(email)).Count(t.Context())
	require.NoError(t, err)
	return n
}

// notMeLink is the token of the "this was not me" link in the newest system mail.
func notMeLink(t *testing.T, env *testhelper.TestEnv) string {
	t.Helper()
	subject, body := lastSystemEmail(t, env)
	require.Contains(t, subject, "provider was linked")
	require.Contains(t, body, "/signin-not-me?token=")
	return tokenFromEmail(t, body)
}

func TestSigninAProvenUserIsLinkedSilentlyAndKeepsThePassword(t *testing.T) {
	env, fake := signinEnv(t)
	registerAndVerify(t, env, "Pat", patEmail, patPassword)
	pat := userByEmail(t, env, patEmail)
	patSession := sessionCookie(postLogin(t, env, patEmail, patPassword).Result())
	require.NotNil(t, patSession)
	b := env.NewRedirectBrowser(t)

	resp := signInWith(t, fake, b, patWithGoogle, "")

	assert.Equal(t, "/", resp.Header.Get("Location"))
	assert.Equal(t, http.StatusOK, meStatus(t, b), "the person is in")
	assert.Equal(t, 1, usersWithEmail(t, env, patEmail), "no duplicate account")
	identities := identitiesOf(t, env, pat)
	require.Len(t, identities, 1)
	assert.Equal(t, patWithGoogle.Subject, identities[0].Subject)
	assert.Equal(t, http.StatusOK, loginStatus(t, env, patEmail, patPassword), "the password still works")
	after := userByEmail(t, env, patEmail)
	assert.Equal(t, pat.SessionEpoch+1, after.SessionEpoch, "a link ends the other sessions")
	assert.Equal(t, http.StatusUnauthorized, sendWithSession(t, env, http.MethodGet, "/site/me", patSession.Value, "").Code, "Pat's earlier session ended")
	assert.Equal(t, "Pat", after.Name, "the account keeps its own name")
}

func TestSigninLinkingNoticesTheOwnerByMail(t *testing.T) {
	env, fake := signinEnv(t)
	registerAndVerify(t, env, "Pat", patEmail, patPassword)
	before := len(env.SystemMail.Messages())

	signInWith(t, fake, env.NewRedirectBrowser(t), patWithGoogle, "")

	messages := env.SystemMail.Messages()
	require.Len(t, messages, before+1)
	notice := messages[len(messages)-1]
	assert.Equal(t, patEmail, notice.To)
	assert.Contains(t, notice.Text, "Google")
	assert.NotEmpty(t, notMeLink(t, env))
}

func TestSigninAKnownIdentityOrANewPersonSendsNoLinkNotice(t *testing.T) {
	env, fake := signinEnv(t)
	before := len(env.SystemMail.Messages())

	signInWith(t, fake, env.NewRedirectBrowser(t), newcomer, "")
	signInWith(t, fake, env.NewRedirectBrowser(t), newcomer, "")

	assert.Len(t, env.SystemMail.Messages(), before)
}

func TestSigninTheNotMeLinkUnlinksTheIdentityAndEndsEverySession(t *testing.T) {
	env, fake := signinEnv(t)
	registerAndVerify(t, env, "Pat", patEmail, patPassword)
	pat := userByEmail(t, env, patEmail)
	patSession := sessionCookie(postLogin(t, env, patEmail, patPassword).Result())
	require.NotNil(t, patSession)
	linked := env.NewRedirectBrowser(t)
	signInWith(t, fake, linked, patWithGoogle, "")
	token := notMeLink(t, env)

	res, err := env.SiteAnonymous(t).SiteAuthSigninNotMe(t.Context(), &siteapi.SiteSigninNotMeInput{Token: token})

	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteAuthSigninNotMeOK{}, res)
	assert.Empty(t, identitiesOf(t, env, pat), "the identity is gone")
	assert.Equal(t, http.StatusUnauthorized, workspacesStatus(t, env, patSession.Value), "the old session ended")
	assert.Equal(t, http.StatusUnauthorized, meStatus(t, linked), "so did the one the link started")
	assert.Equal(t, http.StatusOK, loginStatus(t, env, patEmail, patPassword), "the password still works")
}

func TestSigninTheNotMeLinkWorksOnce(t *testing.T) {
	env, fake := signinEnv(t)
	registerAndVerify(t, env, "Pat", patEmail, patPassword)
	signInWith(t, fake, env.NewRedirectBrowser(t), patWithGoogle, "")
	token := notMeLink(t, env)
	anonymous := env.SiteAnonymous(t)
	_, err := anonymous.SiteAuthSigninNotMe(t.Context(), &siteapi.SiteSigninNotMeInput{Token: token})
	require.NoError(t, err)
	b := env.NewRedirectBrowser(t)
	signed := signInWith(t, fake, b, patWithGoogle, "")
	require.Equal(t, "/", signed.Header.Get("Location"))

	replay, err := anonymous.SiteAuthSigninNotMe(t.Context(), &siteapi.SiteSigninNotMeInput{Token: token})

	require.NoError(t, err)
	assert.IsType(t, &siteapi.ProblemDetails{}, replay)
	assert.Equal(t, http.StatusOK, meStatus(t, b), "the old link ended nothing of the new link")
}

func TestSigninTheNotMeLinkRefusesATokenOfAnotherKind(t *testing.T) {
	env, _ := signinEnv(t)

	res, err := env.SiteAnonymous(t).SiteAuthSigninNotMe(t.Context(), &siteapi.SiteSigninNotMeInput{Token: "not-a-token"})

	require.NoError(t, err)
	assert.IsType(t, &siteapi.ProblemDetails{}, res)
}

func TestSigninLinkingAnUnprovenAddressClearsThePasswordAndEndsTheSessions(t *testing.T) {
	env, fake := signinEnv(t)
	john := userByEmail(t, env, fixtures.OwnerJohnEmail)
	require.Nil(t, john.EmailProvenAt, "the fixture's address was never proven")
	oldSession := sessionCookie(postLogin(t, env, fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword).Result())
	require.NotNil(t, oldSession)
	b := env.NewRedirectBrowser(t)

	resp := signInWith(t, fake, b, johnWithGoogle, "")

	assert.Equal(t, "/", resp.Header.Get("Location"))
	assert.Equal(t, http.StatusOK, meStatus(t, b), "the provider's proof signs in")
	assert.Equal(t, http.StatusUnauthorized, loginStatus(t, env, fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword), "the old password stops working")
	assert.Equal(t, http.StatusUnauthorized, workspacesStatus(t, env, oldSession.Value), "the old session ended")
	after := userByEmail(t, env, fixtures.OwnerJohnEmail)
	assert.Empty(t, after.PasswordHash)
	assert.Greater(t, after.SessionEpoch, john.SessionEpoch)
	assert.NotNil(t, after.EmailProvenAt, "the provider proved the address")
	assert.Equal(t, 1, usersWithEmail(t, env, fixtures.OwnerJohnEmail))
}

func TestSigninLinkingAnUnprovenAddressNoticesTheOwnerToo(t *testing.T) {
	env, fake := signinEnv(t)

	signInWith(t, fake, env.NewRedirectBrowser(t), johnWithGoogle, "")

	assert.NotEmpty(t, notMeLink(t, env))
}

func TestSigninTheNotMeLinkOfAnUnprovenTakeoverLeavesNoWayInButTheReset(t *testing.T) {
	env, fake := signinEnv(t)
	signInWith(t, fake, env.NewRedirectBrowser(t), johnWithGoogle, "")
	token := notMeLink(t, env)

	res, err := env.SiteAnonymous(t).SiteAuthSigninNotMe(t.Context(), &siteapi.SiteSigninNotMeInput{Token: token})

	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteAuthSigninNotMeOK{}, res)
	forgot, err := env.SiteAnonymous(t).SiteAuthForgotPassword(t.Context(), &siteapi.SiteForgotPasswordInput{Email: fixtures.OwnerJohnEmail})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteAuthForgotPasswordAccepted{}, forgot)
	_, body := lastSystemEmail(t, env)
	reset, err := env.SiteAnonymous(t).SiteAuthResetPassword(t.Context(), &siteapi.SiteResetPasswordInput{Token: tokenFromEmail(t, body), Password: "brandnewpass1"})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteAuthResetPasswordOK{}, reset, "the owner takes the account back by resetting the password")
	assert.Equal(t, http.StatusOK, loginStatus(t, env, fixtures.OwnerJohnEmail, "brandnewpass1"))
}

func TestSigninTheVisitorSeesTheSameWhetherOrNotTheAddressExisted(t *testing.T) {
	env, fake := signinEnv(t)
	registerAndVerify(t, env, "Pat", patEmail, patPassword)
	existing := env.NewRedirectBrowser(t)
	fresh := env.NewRedirectBrowser(t)

	linked := signInWith(t, fake, existing, patWithGoogle, "/w/abc/contacts")
	created := signInWith(t, fake, fresh, newcomer, "/w/abc/contacts")

	assert.Equal(t, created.StatusCode, linked.StatusCode)
	assert.Equal(t, created.Header.Get("Location"), linked.Header.Get("Location"))
	assert.Len(t, linked.Cookies(), len(created.Cookies()))
	assert.Equal(t, created.Body, linked.Body)
}

func TestSigninAnUnverifiedUserIsTakenOverAndItsPendingPasswordRejected(t *testing.T) {
	env, fake := signinEnv(t)
	registerPending(t, env, "Squatter", "victim@gmail.com", "squatter-password-1")
	squatter := userByEmail(t, env, "victim@gmail.com")
	require.Nil(t, squatter.EmailVerifiedAt)
	victim := testhelper.SigninIdentity{Subject: "google-subject-of-victim", Email: "victim@gmail.com", EmailVerified: true, Name: "Victim"}
	b := env.NewRedirectBrowser(t)

	resp := signInWith(t, fake, b, victim, "")

	assert.Equal(t, "/", resp.Header.Get("Location"))
	assert.Equal(t, http.StatusOK, meStatus(t, b))
	after := userByEmail(t, env, "victim@gmail.com")
	assert.Equal(t, squatter.ID, after.ID, "the same row, no duplicate")
	assert.NotNil(t, after.EmailVerifiedAt, "the provider's proof makes it Verified")
	assert.NotNil(t, after.EmailProvenAt)
	assert.Empty(t, after.PasswordHash, "the stranger's pending password is gone")
	assert.Equal(t, "Victim", after.Name)
	assert.Equal(t, http.StatusUnauthorized, loginStatus(t, env, "victim@gmail.com", "squatter-password-1"))
	workspaces, err := after.QueryMemberships().Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, workspaces, "the first Workspace is made as a verify link makes it")
}

func TestSigninATakenOverRegistrationKeepsThePendingInvitation(t *testing.T) {
	env, fake := signinEnv(t)
	registerPending(t, env, "Squatter", "victim@gmail.com", "squatter-password-1")
	token := inviteToAcme(t, env, "victim@gmail.com")
	victim := testhelper.SigninIdentity{Subject: "google-subject-of-victim", Email: "victim@gmail.com", EmailVerified: true, Name: "Victim"}
	b := env.NewRedirectBrowser(t)
	signInWith(t, fake, b, victim, "")

	lookup, err := env.SiteAnonymous(t).SitePublicInvitationsLookup(t.Context(), siteapi.SitePublicInvitationsLookupParams{Token: token})

	require.NoError(t, err)
	found, ok := lookup.(*siteapi.SiteInvitationLookupResult)
	require.Truef(t, ok, "got %T", lookup)
	assert.Equal(t, "victim@gmail.com", string(found.Email))
}

func TestSigninATakeoverOfAnUnverifiedAddressSendsNoNotice(t *testing.T) {
	env, fake := signinEnv(t)
	registerPending(t, env, "Squatter", "victim@gmail.com", "squatter-password-1")
	before := len(env.SystemMail.Messages())

	signInWith(t, fake, env.NewRedirectBrowser(t), testhelper.SigninIdentity{Subject: "google-subject-of-victim", Email: "victim@gmail.com", EmailVerified: true, Name: "Victim"}, "")

	assert.Len(t, env.SystemMail.Messages(), before, "the address's owner is the one signing in")
}

func TestSigninALinkedUserWithASecondFactorStillGetsTheChallenge(t *testing.T) {
	env, fake := signinEnv(t)
	sam := testhelper.SigninIdentity{Subject: "another-google-account-of-sam", Email: fixtures.SecondFactorSamEmail, EmailVerified: true, HostedDomain: "globex.test"}

	resp := signInWith(t, fake, env.NewRedirectBrowser(t), sam, "")

	challengeOf(t, resp)
}

func TestTheAddressIsProvenByEveryLinkItIsMailedTo(t *testing.T) {
	t.Run("a verify link", func(t *testing.T) {
		env := testhelper.Setup(t)
		registerAndVerify(t, env, "Pat", patEmail, patPassword)
		assert.NotNil(t, userByEmail(t, env, patEmail).EmailProvenAt)
	})
	t.Run("a registration with verification off proves nothing", func(t *testing.T) {
		env := gateOff(t)
		require.IsType(t, &siteapi.SiteAuthRegisterAccepted{}, registerWithPassword(t, env, "Pat", patEmail, patPassword))
		u := userByEmail(t, env, patEmail)
		assert.NotNil(t, u.EmailVerifiedAt)
		assert.Nil(t, u.EmailProvenAt)
	})
	t.Run("an accepted invitation of a new address", func(t *testing.T) {
		env := testhelper.Setup(t)
		token := inviteToAcme(t, env, "newbie@example.com")
		res, err := env.SiteAnonymous(t).SitePublicInvitationsAccept(t.Context(),
			&siteapi.SiteAcceptInvitationInput{Name: siteapi.NewOptString("Newbie"), Password: siteapi.NewOptString("newbie-password-1")},
			siteapi.SitePublicInvitationsAcceptParams{Token: token})
		require.NoError(t, err)
		require.IsType(t, &siteapi.SitePublicInvitationsAcceptOK{}, res)
		assert.NotNil(t, userByEmail(t, env, "newbie@example.com").EmailProvenAt)
	})
	t.Run("an accepted invitation of a Verified User", func(t *testing.T) {
		env := testhelper.Setup(t)
		token := inviteToAcme(t, env, fixtures.OutsiderOscarEmail)
		res, err := env.SiteActor(t, fixtures.OutsiderOscarEmail).SitePublicInvitationsAccept(t.Context(),
			&siteapi.SiteAcceptInvitationInput{}, siteapi.SitePublicInvitationsAcceptParams{Token: token})
		require.NoError(t, err)
		require.IsType(t, &siteapi.SitePublicInvitationsAcceptOK{}, res)
		assert.NotNil(t, userByEmail(t, env, fixtures.OutsiderOscarEmail).EmailProvenAt)
	})
	t.Run("a confirmed email change", func(t *testing.T) {
		env := testhelper.Setup(t)
		res, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteUserEmailChange(t.Context(), &siteapi.SiteEmailChangeInput{
			NewEmail: "moved@example.com", CurrentPassword: fixtures.OwnerJohnPassword,
		})
		require.NoError(t, err)
		require.IsType(t, &siteapi.SiteUserEmailChangeAccepted{}, res)
		_, body := lastSystemEmail(t, env)
		confirm, err := env.SiteAnonymous(t).SiteAuthConfirmEmailChange(t.Context(),
			&siteapi.SiteConfirmEmailChangeInput{Token: tokenFromEmail(t, body)}, siteapi.SiteAuthConfirmEmailChangeParams{})
		require.NoError(t, err)
		require.IsType(t, &siteapi.SiteAuthConfirmEmailChangeOK{}, confirm)
		assert.NotNil(t, userByEmail(t, env, "moved@example.com").EmailProvenAt)
	})
	t.Run("a reset link", func(t *testing.T) {
		env := testhelper.Setup(t)
		forgot, err := env.SiteAnonymous(t).SiteAuthForgotPassword(t.Context(), &siteapi.SiteForgotPasswordInput{Email: fixtures.OwnerJohnEmail})
		require.NoError(t, err)
		require.IsType(t, &siteapi.SiteAuthForgotPasswordAccepted{}, forgot)
		_, body := lastSystemEmail(t, env)
		reset, err := env.SiteAnonymous(t).SiteAuthResetPassword(t.Context(), &siteapi.SiteResetPasswordInput{Token: tokenFromEmail(t, body), Password: "brandnewpass1"})
		require.NoError(t, err)
		require.IsType(t, &siteapi.SiteAuthResetPasswordOK{}, reset)
		assert.NotNil(t, userByEmail(t, env, fixtures.OwnerJohnEmail).EmailProvenAt)
	})
}

func TestAuditASilentLinkAndItsNotMeUndoNameOnlyTheProvider(t *testing.T) {
	env, fake := signinEnv(t)
	signInWith(t, fake, env.NewRedirectBrowser(t), johnWithGoogle, "")

	linked := entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, events.ActionUserIdentityLink)

	require.Len(t, linked, 1)
	assert.JSONEq(t, `{"provider":"google"}`, diffOf(t, linked[0]))
	assert.NotContains(t, diffOf(t, linked[0]), johnWithGoogle.Subject)

	token := notMeLink(t, env)
	_, err := env.SiteAnonymous(t).SiteAuthSigninNotMe(t.Context(), &siteapi.SiteSigninNotMeInput{Token: token})
	require.NoError(t, err)

	unlinked := entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, events.ActionUserIdentityUnlink)
	require.Len(t, unlinked, 1)
	assert.JSONEq(t, `{"provider":"google"}`, diffOf(t, unlinked[0]))
}

// registerPending registers an address and leaves its verify link unused: an
// Unverified User with a pending password.
func registerPending(t *testing.T, env *testhelper.TestEnv, name, email, password string) {
	t.Helper()
	registerIn(t, env, env.SiteAnonymous(t), name, email, password)
}
