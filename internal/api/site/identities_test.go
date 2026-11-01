package site_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	entuser "github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/ent/useridentity"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/signin"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const (
	identitiesPath = "/site/me/identities"
	linkPath       = "/site/me/identities/google/link"
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

// link runs the whole round trip of linking who to the signed-in person and returns the
// callback's answer.
func link(t *testing.T, fake *testhelper.FakeSignin, b *testhelper.RedirectBrowser, who testhelper.SigninIdentity) testhelper.Reply {
	t.Helper()
	started := b.Get(t, linkPath)
	require.Equal(t, http.StatusFound, started.StatusCode)
	return b.Get(t, fake.Authorize(t, started.Header.Get("Location"), who))
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

func TestIdentitiesLinkNeedsASession(t *testing.T) {
	env, _ := signinEnv(t)

	resp := env.NewRedirectBrowser(t).Get(t, linkPath)

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestIdentitiesAProviderWithAnotherEmailCanBeLinked(t *testing.T) {
	env, fake := signinEnv(t)
	b := signedIn(t, env, fake, mary)
	before := listIdentities(t, b)

	resp := link(t, fake, b, maryAtWork)

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

	link(t, fake, acting, maryAtWork)

	assert.Equal(t, http.StatusOK, meStatus(t, acting))
	assert.Equal(t, http.StatusOK, meStatus(t, other), "a new way to sign in ends no session")
}

func TestIdentitiesLinkingASubjectOfAnotherUserIsRefused(t *testing.T) {
	env, fake := signinEnv(t)
	b := signedIn(t, env, fake, mary)
	before := listIdentities(t, b)
	sams := testhelper.SigninIdentity{Subject: fixtures.SamGoogleIdentitySubject, Email: "sam@gmail.com", EmailVerified: true}

	resp := link(t, fake, b, sams)

	assert.Equal(t, signin.ReasonIdentityInUse, refusalCode(t, resp, settingsPath))
	after := listIdentities(t, b)
	assert.Len(t, after.Items, len(before.Items))
	owner, err := env.DB.UserIdentity.Query().Where(useridentity.Subject(fixtures.SamGoogleIdentitySubject)).QueryUser().Only(t.Context())
	require.NoError(t, err)
	assert.NotEqual(t, fixtures.MemberMaryEmail, owner.Email)
}

func TestIdentitiesLinkingADeniedConsentReportsItOnTheSettingsPage(t *testing.T) {
	env, fake := signinEnv(t)
	b := signedIn(t, env, fake, mary)
	started := b.Get(t, linkPath)

	resp := b.Get(t, fake.Deny(t, started.Header.Get("Location")))

	assert.Equal(t, signin.ReasonDenied, refusalCode(t, resp, settingsPath))
}

func TestIdentitiesALinkFlowOfASessionThatEndedIsRefused(t *testing.T) {
	env, fake := signinEnv(t)
	b := signedIn(t, env, fake, mary)
	started := b.Get(t, linkPath)
	callback := fake.Authorize(t, started.Header.Get("Location"), maryAtWork)
	_, err := env.DB.User.Update().Where(entuser.Email(fixtures.MemberMaryEmail)).AddSessionEpoch(1).Save(t.Context())
	require.NoError(t, err)

	resp := b.Get(t, callback)

	assert.Equal(t, signin.ReasonInvalidState, refusalCode(t, resp, settingsPath))
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
	link(t, fake, acting, second)
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

	resp := b.Delete(t, identitiesPath+"/"+strconv.FormatInt(sams.ID, 10))

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	still, err := env.DB.UserIdentity.Query().Where(useridentity.Subject(fixtures.SamGoogleIdentitySubject)).Exist(t.Context())
	require.NoError(t, err)
	assert.True(t, still)
}
