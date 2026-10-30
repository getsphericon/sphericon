package site_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/consentrecord"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/consentrecords"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A Sign-in provider's sign-up waits for the Sign-up items (ADR 0062): with items in effect the
// callback keeps a pending sign-up and sends the browser to the screen that asks them.

const (
	signinPendingPage = "/signin-pending"
	signinPendingAPI  = "/site/auth/signin-pending"
)

// pendingToken is the signed reference of the screen a callback held pending landed on.
func pendingToken(t *testing.T, resp testhelper.Reply) string {
	t.Helper()
	require.Equal(t, http.StatusFound, resp.StatusCode)
	u, err := url.Parse(resp.Header.Get("Location"))
	require.NoError(t, err)
	require.Equal(t, signinPendingPage, u.Path)
	token := u.Query().Get("token")
	require.NotEmpty(t, token)
	return token
}

// submitPending posts the answers of the screen, as the browser that landed on it.
func submitPending(t *testing.T, b *testhelper.RedirectBrowser, token string, answers ...siteapi.SiteSignupItemAnswer) testhelper.Reply {
	t.Helper()
	body, err := json.Marshal(siteapi.SiteSigninPendingSubmitInput{Token: token, SignupItems: answers})
	require.NoError(t, err)
	return b.PostJSON(t, signinPendingAPI, string(body))
}

func declinePending(t *testing.T, b *testhelper.RedirectBrowser, token string) testhelper.Reply {
	t.Helper()
	body, err := json.Marshal(siteapi.SiteSigninPendingDeclineInput{Token: token})
	require.NoError(t, err)
	return b.PostJSON(t, signinPendingAPI+"/decline", string(body))
}

func readPending(t *testing.T, b *testhelper.RedirectBrowser, token string) testhelper.Reply {
	t.Helper()
	return b.Get(t, signinPendingAPI+"?"+url.Values{"token": {token}}.Encode())
}

// providerScreenAnswers is the User's current answers, each as its key, decision and source.
func providerScreenAnswers(t *testing.T, env *testhelper.TestEnv, email string) []string {
	t.Helper()
	records, err := env.ConsentRecords.Current(t.Context(), userByEmail(t, env, email).ID)
	require.NoError(t, err)
	return lo.Map(records, func(r consentrecords.Record, _ int) string {
		return r.Key + ": " + string(r.Decision) + " (" + string(r.Source) + ")"
	})
}

func TestSigninWithSignupItemsHoldsANewPersonPendingAndCreatesNoUser(t *testing.T) {
	env, fake := signinEnv(t)
	withSignupItems(t, env)
	b := env.NewRedirectBrowser(t)

	resp := signInWith(t, fake, b, newcomer, "")

	token := pendingToken(t, resp)
	assert.Equal(t, http.StatusUnauthorized, meStatus(t, b), "no session exists before the answers")
	assert.False(t, userExists(t, env, newcomer.Email))
	read := readPending(t, b, token)
	require.Equal(t, http.StatusOK, read.StatusCode, read.Body)
	assert.JSONEq(t, `{"provider":"Google","email":"nina@gmail.com","name":"Nina"}`, read.Body)
}

func TestSigninHeldPendingKeepsWhereTheFlowBegan(t *testing.T) {
	env, fake := signinEnv(t)
	withSignupItems(t, env)

	resp := signInWith(t, fake, env.NewRedirectBrowser(t), newcomer, "/w/next")

	u, err := url.Parse(resp.Header.Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "/w/next", u.Query().Get("redirect"))
}

func TestSubmittingTheAnswersSignsThePersonInWithTheirUserIdentityAndWorkspace(t *testing.T) {
	env, fake := signinEnv(t)
	withSignupItems(t, env)
	b := env.NewRedirectBrowser(t)
	token := pendingToken(t, signInWith(t, fake, b, newcomer, ""))

	resp := submitPending(t, b, token, termsTicked, newsUnticked)

	require.Equal(t, http.StatusNoContent, resp.StatusCode, resp.Body)
	assert.Equal(t, http.StatusOK, meStatus(t, b), "the answers start the session")
	nina := userByEmail(t, env, newcomer.Email)
	assert.NotNil(t, nina.EmailVerifiedAt)
	assert.Equal(t, "Nina", nina.Name)
	workspaces, err := nina.QueryMemberships().Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, workspaces, "the usual first Workspace is there to start from")
	assert.Equal(t, []string{
		fixtures.ServiceTermsKey + ": accepted (" + string(consentrecord.SourceProviderScreen) + ")",
		fixtures.OperatorNewsKey + ": declined (" + string(consentrecord.SourceProviderScreen) + ")",
		fixtures.DataNoticeKey + ": shown (" + string(consentrecord.SourceProviderScreen) + ")",
	}, providerScreenAnswers(t, env, newcomer.Email))
	again := env.NewRedirectBrowser(t)
	assert.Equal(t, "/", signInWith(t, fake, again, newcomer, "").Header.Get("Location"),
		"the Linked identity signs in next time, without the items")
}

func TestAPendingSignupIsSubmittedOnce(t *testing.T) {
	env, fake := signinEnv(t)
	withSignupItems(t, env)
	b := env.NewRedirectBrowser(t)
	token := pendingToken(t, signInWith(t, fake, b, newcomer, ""))
	submitPending(t, b, token, termsTicked)

	resp := submitPending(t, env.NewRedirectBrowser(t), token, termsTicked)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestASubmitWithoutTheRequiredAcceptanceIsRefusedAndCanBeCorrected(t *testing.T) {
	env, fake := signinEnv(t)
	withSignupItems(t, env)
	b := env.NewRedirectBrowser(t)
	token := pendingToken(t, signInWith(t, fake, b, newcomer, ""))

	refused := submitPending(t, b, token, newsUnticked)

	require.Equal(t, http.StatusUnprocessableEntity, refused.StatusCode, refused.Body)
	var problem struct {
		Errors map[string][]string `json:"errors"`
	}
	require.NoError(t, json.Unmarshal([]byte(refused.Body), &problem))
	assert.Equal(t, map[string][]string{
		"signupItems." + fixtures.ServiceTermsKey: {"accept the Terms of service to sign up"},
	}, problem.Errors)
	assert.False(t, userExists(t, env, newcomer.Email))
	assert.Equal(t, http.StatusUnauthorized, meStatus(t, b))

	corrected := submitPending(t, b, token, termsTicked, newsUnticked)

	assert.Equal(t, http.StatusNoContent, corrected.StatusCode, corrected.Body)
}

func TestDecliningLeavesNoUserAndNoPendingSignup(t *testing.T) {
	env, fake := signinEnv(t)
	withSignupItems(t, env)
	b := env.NewRedirectBrowser(t)
	token := pendingToken(t, signInWith(t, fake, b, newcomer, ""))

	resp := declinePending(t, b, token)

	require.Equal(t, http.StatusNoContent, resp.StatusCode, resp.Body)
	assert.False(t, userExists(t, env, newcomer.Email))
	assert.Equal(t, http.StatusBadRequest, readPending(t, b, token).StatusCode, "the pending sign-up is gone")
	assert.Equal(t, http.StatusBadRequest, submitPending(t, b, token, termsTicked).StatusCode)
	assert.Equal(t, http.StatusNoContent, declinePending(t, b, token).StatusCode, "declining again changes nothing")
}

func TestAnExpiredPendingSignupCanNoLongerBeSubmitted(t *testing.T) {
	env, fake, clock := signinEnvOnClock(t)
	withSignupItems(t, env)
	b := env.NewRedirectBrowser(t)
	token := pendingToken(t, signInWith(t, fake, b, newcomer, ""))
	clock.t = clock.t.Add(11 * time.Minute)

	resp := submitPending(t, b, token, termsTicked)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.False(t, userExists(t, env, newcomer.Email))
	assert.Equal(t, http.StatusBadRequest, readPending(t, b, token).StatusCode)
}

func TestAForgedPendingReferenceIsRefused(t *testing.T) {
	env, _ := signinEnv(t)
	withSignupItems(t, env)
	b := env.NewRedirectBrowser(t)

	resp := submitPending(t, b, "not-a-reference", termsTicked)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestAKnownLinkedIdentitySignsInWithoutTheItems(t *testing.T) {
	env, fake := signinEnv(t)
	withSignupItems(t, env)
	b := env.NewRedirectBrowser(t)
	maryNow := testhelper.SigninIdentity{Subject: fixtures.MaryGoogleIdentitySubject, Email: "mary@elsewhere.test"}

	resp := signInWith(t, fake, b, maryNow, "")

	assert.Equal(t, "/", resp.Header.Get("Location"))
	assert.Equal(t, http.StatusOK, meStatus(t, b))
}

func TestAnInvitationSignupWaitsForTheItemsAndAcceptsTheInvitationOnSubmit(t *testing.T) {
	env, fake := signinEnv(t)
	invite := inviteAdminToAcme(t, env, newcomer.Email)
	withSignupItems(t, env)
	b := env.NewRedirectBrowser(t)
	token := pendingToken(t, signInToAccept(t, fake, b, newcomer, invite))
	assert.Equal(t, 1, pendingInvitations(t, env, newcomer.Email), "nothing is accepted before the answers")

	resp := submitPending(t, b, token, termsTicked)

	require.Equal(t, http.StatusNoContent, resp.StatusCode, resp.Body)
	assert.Equal(t, http.StatusOK, meStatus(t, b))
	assert.Equal(t, "admin", acmeRoleOf(t, env, newcomer.Email), "the Membership has the invited Role")
	assert.Zero(t, pendingInvitations(t, env, newcomer.Email), "the Invitation is accepted")
}

// victimWithGoogle is the owner of victim@gmail.com signing in with Google, after a stranger
// signed up with the address and never verified it.
var victimWithGoogle = testhelper.SigninIdentity{Subject: "google-subject-of-victim", Email: "victim@gmail.com", EmailVerified: true, Name: "Victim"}

// squattedBeforeTheItems is an Unverified User of victim@gmail.com with a pending password, signed
// up before the Instance had Sign-up items in effect, which then are.
func squattedBeforeTheItems(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	signUpPending(t, env, "Squatter", victimWithGoogle.Email, "squatter-password-1")
	withSignupItems(t, env)
}

func TestATakeoverWaitsForTheItemsAndLeavesTheUnverifiedUserAsItWas(t *testing.T) {
	env, fake := signinEnv(t)
	squattedBeforeTheItems(t, env)
	b := env.NewRedirectBrowser(t)

	pendingToken(t, signInWith(t, fake, b, victimWithGoogle, ""))

	assert.Equal(t, http.StatusUnauthorized, meStatus(t, b), "no session exists before the answers")
	squatter := userByEmail(t, env, victimWithGoogle.Email)
	assert.Nil(t, squatter.EmailVerifiedAt, "the User is not taken over before the answers")
	assert.NotEmpty(t, squatter.PasswordHash)
	assert.Equal(t, "Squatter", squatter.Name)
}

func TestSubmittingTheAnswersTakesTheUnverifiedUserOverWithItsConsentRecords(t *testing.T) {
	env, fake := signinEnv(t)
	squattedBeforeTheItems(t, env)
	squatter := userByEmail(t, env, victimWithGoogle.Email)
	b := env.NewRedirectBrowser(t)
	token := pendingToken(t, signInWith(t, fake, b, victimWithGoogle, ""))

	resp := submitPending(t, b, token, termsTicked, newsUnticked)

	require.Equal(t, http.StatusNoContent, resp.StatusCode, resp.Body)
	assert.Equal(t, http.StatusOK, meStatus(t, b), "the answers start the session")
	after := userByEmail(t, env, victimWithGoogle.Email)
	assert.Equal(t, squatter.ID, after.ID, "the same row, no duplicate")
	assert.NotNil(t, after.EmailVerifiedAt, "the provider's proof makes it Verified")
	assert.Empty(t, after.PasswordHash, "the stranger's pending password is gone")
	assert.Equal(t, "Victim", after.Name)
	assert.Equal(t, []string{
		fixtures.ServiceTermsKey + ": accepted (" + string(consentrecord.SourceProviderScreen) + ")",
		fixtures.OperatorNewsKey + ": declined (" + string(consentrecord.SourceProviderScreen) + ")",
		fixtures.DataNoticeKey + ": shown (" + string(consentrecord.SourceProviderScreen) + ")",
	}, providerScreenAnswers(t, env, victimWithGoogle.Email))
	again := env.NewRedirectBrowser(t)
	assert.Equal(t, "/", signInWith(t, fake, again, victimWithGoogle, "").Header.Get("Location"),
		"the Linked identity signs in next time, without the items")
}

func TestATakeoverIsGoneOnceThePersonWhoSignedUpVerifiesTheAddress(t *testing.T) {
	env, fake := signinEnv(t)
	squattedBeforeTheItems(t, env)
	b := env.NewRedirectBrowser(t)
	token := pendingToken(t, signInWith(t, fake, b, victimWithGoogle, ""))
	squatter := userByEmail(t, env, victimWithGoogle.Email)
	require.NoError(t, env.FeedDB.User.UpdateOneID(squatter.ID).SetEmailVerifiedAt(time.Now()).Exec(t.Context()))

	resp := submitPending(t, b, token, termsTicked)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, resp.Body)
	stillHers := userByEmail(t, env, victimWithGoogle.Email)
	assert.NotEmpty(t, stillHers.PasswordHash, "the Verified User is left alone")
	assert.Equal(t, http.StatusUnauthorized, meStatus(t, b))
}
