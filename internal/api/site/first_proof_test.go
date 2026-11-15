package site_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The first proof of an address drops the ways into the account that were set up before it:
// whoever proves the address is the one who controls the account (ADR 0170). John's fixture
// address is Verified but was never proven, so whoever knows his password stands for a squatter.

// squatterWithGoogle is a Google account of someone who is not the owner of John's address.
var squatterWithGoogle = testhelper.SigninIdentity{
	Subject: "google-subject-of-a-squatter", Email: "squatter@elsewhere.test", EmailVerified: true, Name: "Squatter",
}

// squatterSession is a browser signed in to John's account with its password, as whoever set it
// before the address was proven could be.
func squatterSession(t *testing.T, env *testhelper.TestEnv) *testhelper.RedirectBrowser {
	t.Helper()
	john := userByEmail(t, env, fixtures.OwnerJohnEmail)
	require.Nil(t, john.EmailProvenAt, "the fixture's address was never proven")
	b := env.NewRedirectBrowser(t)
	login := b.PostJSON(t, loginPath, loginBody(fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword))
	require.Equal(t, http.StatusOK, login.StatusCode)
	return b
}

// squatterLinksGoogle links the squatter's Google account to John's account from the squatter's
// session.
func squatterLinksGoogle(t *testing.T, fake *testhelper.FakeSignin, squatter *testhelper.RedirectBrowser) {
	t.Helper()
	link(t, fake, squatter, squatterWithGoogle)
	linked := listIdentities(t, squatter)
	require.Len(t, linked.Items, 1, "the squatter's Google account is linked to John's")
}

// enrollFactor enrolls a TOTP Second factor on John's account with its password.
func enrollFactor(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	john := env.SiteActor(t, fixtures.OwnerJohnEmail)
	password := siteapi.NewOptString(fixtures.OwnerJohnPassword)
	started, err := john.SiteSecondFactorStartEnrollment(t.Context(), &siteapi.SiteSecondFactorStartInput{CurrentPassword: password})
	require.NoError(t, err)
	code, err := totp.GenerateCode(started.Secret, time.Now())
	require.NoError(t, err)
	_, err = john.SiteSecondFactorConfirmEnrollment(t.Context(), &siteapi.SiteSecondFactorConfirmInput{
		EnrollmentId: started.EnrollmentId, CurrentPassword: password, Code: code,
	})
	require.NoError(t, err)
}

// meEmail is the login email of the account the browser's session is in, "" without a session.
func meEmail(t *testing.T, b *testhelper.RedirectBrowser) string {
	t.Helper()
	resp := b.Get(t, "/site/me")
	var me struct {
		Email string `json:"email"`
	}
	require.NoError(t, json.Unmarshal([]byte(resp.Body), &me), "a Problem without a session decodes too")
	return me.Email
}

func TestAProviderProofDropsTheIdentitiesAndSessionsOfBeforeIt(t *testing.T) {
	env, fake := signinEnv(t)
	squatter := squatterSession(t, env)
	squatterLinksGoogle(t, fake, squatter)

	owner := signedIn(t, env, fake, johnWithGoogle)

	identities := listIdentities(t, owner)
	squatterNow := meEmail(t, squatter)
	again := env.NewRedirectBrowser(t)
	signInWith(t, fake, again, squatterWithGoogle, "")
	landed := meEmail(t, again)
	require.Len(t, identities.Items, 1, "only the owner's own identity is left")
	assert.Empty(t, squatterNow, "the squatter's session ended")
	assert.NotEqual(t, fixtures.OwnerJohnEmail, landed, "the squatter's Google account no longer signs in to John's account")
}

func TestAProviderProofDropsTheSecondFactorEnrolledBeforeIt(t *testing.T) {
	env, fake := signinEnv(t)
	enrollFactor(t, env)
	owner := env.NewRedirectBrowser(t)

	resp := signInWith(t, fake, owner, johnWithGoogle, "")

	landing := resp.Header.Get("Location")
	email := meEmail(t, owner)
	assert.Equal(t, "/", landing, "no second step: the factor was not the owner's")
	assert.Equal(t, fixtures.OwnerJohnEmail, email)
}

func TestAProviderProofRevokesThePersonalTokensAndConnectionsOfBeforeIt(t *testing.T) {
	env, fake := signinEnv(t)
	minted := mintedPersonal(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, siteapi.PersonalScopeContactsRead)
	script := env.ExternalWithToken(t, minted.Token)
	app := env.ExternalWithToken(t, env.ConnectionBearer(t, fixtures.OwnerJohnEmail, apitokens.ScopeContactsRead))
	_, scriptBefore := script.ContactsList(t.Context(), externalapi.ContactsListParams{})
	_, appBefore := app.ContactsList(t.Context(), externalapi.ContactsListParams{})
	require.NoError(t, scriptBefore, "the Personal token works before the proof")
	require.NoError(t, appBefore, "the Connection works before the proof")

	signedIn(t, env, fake, johnWithGoogle)

	_, scriptAfter := script.ContactsList(t.Context(), externalapi.ContactsListParams{})
	_, appAfter := app.ContactsList(t.Context(), externalapi.ContactsListParams{})
	testhelper.ExternalRefusal(t, scriptAfter, http.StatusUnauthorized)
	testhelper.ExternalRefusal(t, appAfter, http.StatusUnauthorized)
}

// A reset link mailed to the address is its first proof too: the owner who follows it gets the
// account without what was set up before.
func TestAResetLinkProofDropsWhatWasSetUpBeforeIt(t *testing.T) {
	env, fake := signinEnv(t)
	squatter := squatterSession(t, env)
	squatterLinksGoogle(t, fake, squatter)
	enrollFactor(t, env)
	minted := mintedPersonal(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, siteapi.PersonalScopeContactsRead)

	resetPassword(t, env, fixtures.OwnerJohnEmail)

	owner := env.NewRedirectBrowser(t)
	owner.PostJSON(t, loginPath, loginBody(fixtures.OwnerJohnEmail, "brandnewpass1"))
	email := meEmail(t, owner)
	identities := listIdentities(t, owner)
	_, tokenErr := env.ExternalWithToken(t, minted.Token).ContactsList(t.Context(), externalapi.ContactsListParams{})
	assert.Equal(t, fixtures.OwnerJohnEmail, email, "the new password signs in with no second step")
	assert.Empty(t, identities.Items)
	testhelper.ExternalRefusal(t, tokenErr, http.StatusUnauthorized)
}

// Only the first proof drops anything: a reset link of an address proven before is a plain password
// change, and Sam keeps his Second factor.
func TestAResetLinkOfAProvenAddressKeepsWhatWasSetUp(t *testing.T) {
	env := testhelper.Setup(t)
	sam := userByEmail(t, env, fixtures.SecondFactorSamEmail)
	require.NotNil(t, sam.EmailProvenAt, "the fixture's address is proven")

	resetPassword(t, env, fixtures.SecondFactorSamEmail)

	rec := postLogin(t, env, fixtures.SecondFactorSamEmail, "brandnewpass1")
	var res loginRes
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	assert.Equal(t, "challenge", res.Outcome, "the second step is still asked")
}
