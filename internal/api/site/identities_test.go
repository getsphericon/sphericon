package site_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	entuser "github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/ent/useridentity"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/identity/signinprovider"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const (
	identitiesPath = "/site/me/identities"
	linkPath       = "/site/me/identities/link"
	settingsPath   = "/account/security"
)

// mary signs in with the Google account the fixtures link to her (she also has a password).
var mary = testhelper.SigninIdentity{Subject: fixtures.MaryGoogleIdentitySubject, Email: fixtures.MemberMaryEmail, EmailVerified: true, HostedDomain: "acme.test"}

// maryAtWork is another Google account of the same person, with another address.
var maryAtWork = testhelper.SigninIdentity{Subject: "google-subject-of-mary-at-work", Email: "mary@work.test", EmailVerified: true, HostedDomain: "work.test"}

type linkedIdentities struct {
	Items []struct {
		ID          string `json:"id"`
		Provider    string `json:"provider"`
		DisplayName string `json:"displayName"`
	} `json:"items"`
	PasswordSet        bool `json:"passwordSet"`
	IdentityProofFresh bool `json:"identityProofFresh"`
}

// signedIn is a browser holding a session of the person who signed in with who.
func signedIn(t *testing.T, env *testhelper.TestEnv, fake *testhelper.FakeSignin, who testhelper.SigninIdentity) *testhelper.RedirectBrowser {
	t.Helper()
	b := env.NewRedirectBrowser(t)
	signInWith(t, fake, b, who, "")
	require.Equal(t, http.StatusOK, meStatus(t, b))
	return b
}

func listIdentities(t *testing.T, b *testhelper.RedirectBrowser) linkedIdentities {
	t.Helper()
	resp := b.Get(t, identitiesPath)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var list linkedIdentities
	require.NoError(t, json.Unmarshal([]byte(resp.Body), &list))
	return list
}

// startLink asks to link a provider account of the given provider to the signed-in person,
// proving it with the password typed ("" for none).
func startLink(t *testing.T, b *testhelper.RedirectBrowser, provider, password string) testhelper.Reply {
	t.Helper()
	body, err := json.Marshal(struct {
		Provider        string `json:"provider"`
		CurrentPassword string `json:"currentPassword,omitempty"`
	}{provider, password})
	require.NoError(t, err)
	return b.PostJSON(t, linkPath, string(body))
}

// providerURL is where a link that started sends the browser: the provider's authorization URL.
func providerURL(t *testing.T, started testhelper.Reply) string {
	t.Helper()
	require.Equal(t, http.StatusOK, started.StatusCode, started.Body)
	var answer struct {
		RedirectURL string `json:"redirectUrl"`
	}
	require.NoError(t, json.Unmarshal([]byte(started.Body), &answer))
	return answer.RedirectURL
}

// link runs the whole round trip of linking who to the signed-in person, who proves it with the
// password typed ("" for a person with none, whose session holds a fresh proof), and returns the
// callback's answer.
func link(t *testing.T, fake *testhelper.FakeSignin, b *testhelper.RedirectBrowser, who testhelper.SigninIdentity, password string) testhelper.Reply {
	t.Helper()
	return b.Get(t, fake.Authorize(t, providerURL(t, startLink(t, b, "google", password)), who))
}

func TestIdentitiesListShowsTheProvidersLinked(t *testing.T) {
	env, fake := signinEnv(t)
	b := signedIn(t, env, fake, mary)

	list := listIdentities(t, b)

	require.Len(t, list.Items, 1)
	assert.Equal(t, "google", list.Items[0].Provider)
	assert.Equal(t, "Google", list.Items[0].DisplayName)
}

func TestIdentitiesListNeedsASession(t *testing.T) {
	env, _ := signinEnv(t)

	resp := env.NewRedirectBrowser(t).Get(t, identitiesPath)

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestIdentitiesListSaysWhatProvesTheUserForALink(t *testing.T) {
	env, fake := signinEnv(t)

	withPassword := listIdentities(t, signedIn(t, env, fake, mary))
	without := listIdentities(t, signedIn(t, env, fake, newcomer))

	assert.True(t, withPassword.PasswordSet)
	assert.False(t, without.PasswordSet)
	assert.True(t, without.IdentityProofFresh, "the provider login just made stands in for the password")
}

func TestIdentitiesLinkNeedsASession(t *testing.T) {
	env, _ := signinEnv(t)

	resp := startLink(t, env.NewRedirectBrowser(t), "google", "")

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// ADR 0176: a link asks for the password of a User who has one, as adding a Passkey does, even
// right after a provider login.
func TestIdentitiesLinkingIsRefusedWithoutThePassword(t *testing.T) {
	env, fake := signinEnv(t)
	b := signedIn(t, env, fake, mary)
	before := listIdentities(t, b)

	missing := startLink(t, b, "google", "")
	wrong := startLink(t, b, "google", "not the password")

	assert.Equal(t, http.StatusForbidden, missing.StatusCode)
	assert.Equal(t, http.StatusForbidden, wrong.StatusCode)
	after := listIdentities(t, b)
	assert.Len(t, after.Items, len(before.Items))
	assert.Empty(t, env.SystemMail.Messages())
}

func TestIdentitiesLinkingAProviderTheInstanceDoesNotOfferIsNotFound(t *testing.T) {
	env, fake := signinEnv(t)
	b := signedIn(t, env, fake, mary)

	resp := startLink(t, b, "myspace", fixtures.MemberMaryPassword)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// ADR 0176: every link tells the owner, and the notice's link undoes it.
func TestIdentitiesLinkingMailsTheOwnerANoticeWhoseLinkUndoesIt(t *testing.T) {
	env, fake := signinEnv(t)
	acting := signedIn(t, env, fake, mary)
	other := signedIn(t, env, fake, mary)
	before := len(env.SystemMail.Messages())
	link(t, fake, acting, maryAtWork, fixtures.MemberMaryPassword)
	messages := env.SystemMail.Messages()
	require.Len(t, messages, before+1)
	assert.Equal(t, fixtures.MemberMaryEmail, messages[len(messages)-1].To)
	token := notMeLink(t, env)

	err := env.SiteAnonymous(t).SiteAuthSigninNotMe(t.Context(), &siteapi.SiteSigninNotMeInput{Token: token})

	require.NoError(t, err)
	linked, err := env.DB.UserIdentity.Query().Where(useridentity.Subject(maryAtWork.Subject)).Exist(t.Context())
	require.NoError(t, err)
	assert.False(t, linked, "the identity is gone")
	assert.Equal(t, http.StatusUnauthorized, meStatus(t, acting), "the session that linked it ended")
	assert.Equal(t, http.StatusUnauthorized, meStatus(t, other))
}

func TestIdentitiesLinkingAProviderAccountTheUserHoldsMailsNothing(t *testing.T) {
	env, fake := signinEnv(t)
	b := signedIn(t, env, fake, mary)
	before := len(env.SystemMail.Messages())

	resp := link(t, fake, b, mary, fixtures.MemberMaryPassword)

	assert.Equal(t, settingsPath, resp.Header.Get("Location"))
	assert.Len(t, env.SystemMail.Messages(), before)
}

func TestIdentitiesAProviderWithAnotherEmailCanBeLinked(t *testing.T) {
	env, fake := signinEnv(t)
	b := signedIn(t, env, fake, mary)
	before := listIdentities(t, b)

	resp := link(t, fake, b, maryAtWork, fixtures.MemberMaryPassword)

	assert.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Equal(t, settingsPath, resp.Header.Get("Location"))
	after := listIdentities(t, b)
	assert.Len(t, after.Items, len(before.Items)+1)
	assert.Equal(t, http.StatusOK, meStatus(t, b), "the acting session continues")
	elsewhere, err := env.DB.User.Query().Where(entuser.Email(maryAtWork.Email)).Exist(t.Context())
	require.NoError(t, err)
	assert.False(t, elsewhere, "no account was made for the other address")
	signedInAgain := env.NewRedirectBrowser(t)
	signInWith(t, fake, signedInAgain, maryAtWork, "")
	assert.Equal(t, http.StatusOK, meStatus(t, signedInAgain), "the linked account signs in to the same User")
	signedInUser, err := env.DB.UserIdentity.Query().Where(useridentity.Subject(maryAtWork.Subject)).QueryUser().Only(t.Context())
	require.NoError(t, err)
	assert.Equal(t, fixtures.MemberMaryEmail, signedInUser.Email)
}

func TestIdentitiesLinkingKeepsTheOtherSessions(t *testing.T) {
	env, fake := signinEnv(t)
	acting := signedIn(t, env, fake, mary)
	other := signedIn(t, env, fake, mary)

	link(t, fake, acting, maryAtWork, fixtures.MemberMaryPassword)

	assert.Equal(t, http.StatusOK, meStatus(t, acting))
	assert.Equal(t, http.StatusOK, meStatus(t, other), "a new way to sign in ends no session")
}

func TestIdentitiesLinkingASubjectOfAnotherUserIsRefused(t *testing.T) {
	env, fake := signinEnv(t)
	b := signedIn(t, env, fake, mary)
	before := listIdentities(t, b)
	sams := testhelper.SigninIdentity{Subject: fixtures.SamGoogleIdentitySubject, Email: "sam@gmail.com", EmailVerified: true}

	resp := link(t, fake, b, sams, fixtures.MemberMaryPassword)

	assert.Equal(t, signinprovider.ReasonIdentityInUse, refusalCode(t, resp, settingsPath))
	after := listIdentities(t, b)
	assert.Len(t, after.Items, len(before.Items))
	owner, err := env.DB.UserIdentity.Query().Where(useridentity.Subject(fixtures.SamGoogleIdentitySubject)).QueryUser().Only(t.Context())
	require.NoError(t, err)
	assert.NotEqual(t, fixtures.MemberMaryEmail, owner.Email)
}

func TestIdentitiesLinkingADeniedConsentReportsItOnTheSettingsPage(t *testing.T) {
	env, fake := signinEnv(t)
	b := signedIn(t, env, fake, mary)
	started := startLink(t, b, "google", fixtures.MemberMaryPassword)

	resp := b.Get(t, fake.Deny(t, providerURL(t, started)))

	assert.Equal(t, signinprovider.ReasonDenied, refusalCode(t, resp, settingsPath))
}

func TestIdentitiesALinkFlowOfASessionThatEndedIsRefused(t *testing.T) {
	env, fake := signinEnv(t)
	b := signedIn(t, env, fake, mary)
	started := startLink(t, b, "google", fixtures.MemberMaryPassword)
	callback := fake.Authorize(t, providerURL(t, started), maryAtWork)
	_, err := env.DB.User.Update().Where(entuser.Email(fixtures.MemberMaryEmail)).AddSessionEpoch(1).Save(t.Context())
	require.NoError(t, err)

	resp := b.Get(t, callback)

	assert.Equal(t, signinprovider.ReasonInvalidState, refusalCode(t, resp, settingsPath))
	linked, err := env.DB.UserIdentity.Query().Where(useridentity.Subject(maryAtWork.Subject)).Exist(t.Context())
	require.NoError(t, err)
	assert.False(t, linked)
}

func TestIdentitiesUnlinkingTheLastWayToSignInIsRefused(t *testing.T) {
	env, fake := signinEnv(t)
	b := signedIn(t, env, fake, newcomer)
	list := listIdentities(t, b)
	require.Len(t, list.Items, 1)

	resp := b.Delete(t, identitiesPath+"/"+list.Items[0].ID)

	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	after := listIdentities(t, b)
	assert.Len(t, after.Items, 1)
}

func TestIdentitiesUnlinkingWorksWhenAnotherWayRemainsAndEndsTheOtherSessions(t *testing.T) {
	env, fake := signinEnv(t)
	acting := signedIn(t, env, fake, newcomer)
	other := signedIn(t, env, fake, newcomer)
	second := testhelper.SigninIdentity{Subject: "google-subject-of-nina-at-work", Email: "nina@work.test", EmailVerified: true, HostedDomain: "work.test"}
	link(t, fake, acting, second, "")
	list := listIdentities(t, acting)
	require.Len(t, list.Items, 2)

	resp := acting.Delete(t, identitiesPath+"/"+list.Items[0].ID)

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	after := listIdentities(t, acting)
	assert.Len(t, after.Items, 1)
	assert.Equal(t, http.StatusOK, meStatus(t, acting), "the acting session continues")
	assert.Equal(t, http.StatusUnauthorized, meStatus(t, other))
}

func TestIdentitiesAPersonWithAPasswordCanUnlinkTheOnlyIdentity(t *testing.T) {
	env, fake := signinEnv(t)
	b := signedIn(t, env, fake, mary)
	list := listIdentities(t, b)
	require.Len(t, list.Items, 1)

	resp := b.Delete(t, identitiesPath+"/"+list.Items[0].ID)

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	after := listIdentities(t, b)
	assert.Empty(t, after.Items)
}

func TestIdentitiesUnlinkingAnotherUsersIdentityIsNotFound(t *testing.T) {
	env, fake := signinEnv(t)
	b := signedIn(t, env, fake, mary)
	sams, err := env.DB.UserIdentity.Query().Where(useridentity.Subject(fixtures.SamGoogleIdentitySubject)).Only(t.Context())
	require.NoError(t, err)

	resp := b.Delete(t, identitiesPath+"/"+testhelper.ID(sams.ID))

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	still, err := env.DB.UserIdentity.Query().Where(useridentity.Subject(fixtures.SamGoogleIdentitySubject)).Exist(t.Context())
	require.NoError(t, err)
	assert.True(t, still)
}
