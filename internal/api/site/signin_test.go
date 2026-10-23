package site_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	entuser "github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/ent/useridentity"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/signin"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const (
	signinStartPath = "/site/auth/signin/google/start"
	loginErrorPath  = "/login"
)

// newcomer is a person with a Gmail address Google vouches for, who has no account yet.
var newcomer = testhelper.SigninIdentity{Subject: "google-subject-of-nina", Email: "nina@gmail.com", EmailVerified: true, Name: "Nina"}

func signinEnv(t *testing.T, opts ...testhelper.Option) (*testhelper.TestEnv, *testhelper.FakeSignin) {
	t.Helper()
	fake := testhelper.NewFakeSignin(t)
	return testhelper.Setup(t, append([]testhelper.Option{testhelper.WithSignin(fake)}, opts...)...), fake
}

// signinEnvOnClock is signinEnv on a clock the test owns, for the code a Second factor asks for.
func signinEnvOnClock(t *testing.T, opts ...testhelper.Option) (*testhelper.TestEnv, *testhelper.FakeSignin, *movableClock) {
	t.Helper()
	c := &movableClock{t: time.Now()}
	env, fake := signinEnv(t, append([]testhelper.Option{testhelper.WithClock(c.now)}, opts...)...)
	return env, fake, c
}

// startSignin is the first hop: the browser asks `start` and is sent to the provider.
func startSignin(t *testing.T, b *testhelper.RedirectBrowser, returnTo string) string {
	t.Helper()
	target := signinStartPath
	if returnTo != "" {
		target += "?" + url.Values{"return_to": {returnTo}}.Encode()
	}
	resp := b.Get(t, target)
	require.Equal(t, http.StatusFound, resp.StatusCode)
	return resp.Header.Get("Location")
}

// signInWith runs the whole round trip for who and returns the callback's answer.
func signInWith(t *testing.T, fake *testhelper.FakeSignin, b *testhelper.RedirectBrowser, who testhelper.SigninIdentity, returnTo string) testhelper.Reply {
	t.Helper()
	authURL := startSignin(t, b, returnTo)
	return b.Get(t, fake.Authorize(t, authURL, who))
}

// refusalCode is the Reason in the `error` of the page (the login page, or the settings page
// for a link flow) a refused callback lands on.
func refusalCode(t *testing.T, resp testhelper.Reply, page string) signin.Reason {
	t.Helper()
	require.Equal(t, http.StatusFound, resp.StatusCode)
	u, err := url.Parse(resp.Header.Get("Location"))
	require.NoError(t, err)
	require.Equal(t, page, u.Path)
	return signin.Reason(u.Query().Get("error"))
}

// meStatus is what /site/me answers the browser: 200 when its session is live.
func meStatus(t *testing.T, b *testhelper.RedirectBrowser) int {
	t.Helper()
	return b.Get(t, "/site/me").StatusCode
}

func TestSigninANewPersonIsSignedUpVerifiedWithNoMail(t *testing.T) {
	env, fake := signinEnv(t)
	b := env.NewRedirectBrowser(t)
	before := len(env.SystemMail.Messages())

	resp := signInWith(t, fake, b, newcomer, "")

	assert.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Equal(t, "/", resp.Header.Get("Location"))
	assert.Equal(t, http.StatusOK, meStatus(t, b), "the callback started a session")
	u, err := env.DB.User.Query().Where(entuser.Email(newcomer.Email)).Only(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, u.EmailVerifiedAt, "the email is Verified")
	assert.NotNil(t, u.EmailProvenAt, "the provider proved it")
	assert.Empty(t, u.PasswordHash, "no password is made up")
	assert.Equal(t, "Nina", u.Name)
	workspaces, err := u.QueryMemberships().Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, workspaces, "the usual first Workspace is there to start from")
	assert.Len(t, env.SystemMail.Messages(), before, "no mail is sent")
}

func TestSigninWithoutANameUsesTheLocalPartOfTheEmail(t *testing.T) {
	env, fake := signinEnv(t)
	who := newcomer
	who.Name = ""

	signInWith(t, fake, env.NewRedirectBrowser(t), who, "")

	u, err := env.DB.User.Query().Where(entuser.Email(newcomer.Email)).Only(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "nina", u.Name)
}

func TestSigninTheSamePersonSignsInAgainBySubjectAlsoAfterChangingTheEmail(t *testing.T) {
	env, fake := signinEnv(t)
	signInWith(t, fake, env.NewRedirectBrowser(t), newcomer, "")
	first, err := env.DB.User.Query().Where(entuser.Email(newcomer.Email)).Only(t.Context())
	require.NoError(t, err)
	moved := newcomer
	moved.Email = "nina.new@gmail.com"
	again := env.NewRedirectBrowser(t)

	resp := signInWith(t, fake, again, moved, "")

	assert.Equal(t, "/", resp.Header.Get("Location"))
	assert.Equal(t, http.StatusOK, meStatus(t, again))
	identity, err := env.DB.UserIdentity.Query().Where(useridentity.Provider("google"), useridentity.Subject(newcomer.Subject)).Only(t.Context())
	require.NoError(t, err)
	assert.Equal(t, first.ID, identity.UserID)
	exists, err := env.DB.User.Query().Where(entuser.Email(moved.Email)).Exist(t.Context())
	require.NoError(t, err)
	assert.False(t, exists, "no second account was made for the new address")
}

func TestSigninALinkedIdentityOfAFixtureUserSignsIn(t *testing.T) {
	env, fake := signinEnv(t)
	b := env.NewRedirectBrowser(t)
	maryNow := testhelper.SigninIdentity{Subject: fixtures.MaryGoogleIdentitySubject, Email: "mary@elsewhere.test", EmailVerified: false}

	resp := signInWith(t, fake, b, maryNow, "")

	assert.Equal(t, "/", resp.Header.Get("Location"), "the subject alone is enough, whatever the provider says of the email")
	assert.Equal(t, http.StatusOK, meStatus(t, b))
}

func TestSigninRefusesWhatTheProviderDoesNotProve(t *testing.T) {
	cases := map[string]struct {
		who  testhelper.SigninIdentity
		code signin.Reason
	}{
		"an unverified email":               {testhelper.SigninIdentity{Subject: "s1", Email: "a@gmail.com", EmailVerified: false}, signin.ReasonEmailUnverified},
		"a non-Gmail address without hd":    {testhelper.SigninIdentity{Subject: "s2", Email: "a@corp.test", EmailVerified: true}, signin.ReasonEmailNotTrusted},
		"no email at all":                   {testhelper.SigninIdentity{Subject: "s3"}, signin.ReasonEmailUnverified},
		"an unverified hosted-domain email": {testhelper.SigninIdentity{Subject: "s4", Email: "a@corp.test", EmailVerified: false, HostedDomain: "corp.test"}, signin.ReasonEmailUnverified},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			env, fake := signinEnv(t)
			b := env.NewRedirectBrowser(t)

			resp := signInWith(t, fake, b, tc.who, "")

			assert.Equal(t, tc.code, refusalCode(t, resp, loginErrorPath))
			assert.Equal(t, http.StatusUnauthorized, meStatus(t, b), "no session started")
		})
	}
}

func TestSigninAcceptsAVerifiedAddressOfAHostedDomain(t *testing.T) {
	env, fake := signinEnv(t)
	b := env.NewRedirectBrowser(t)

	resp := signInWith(t, fake, b, testhelper.SigninIdentity{Subject: "s5", Email: "a@corp.test", EmailVerified: true, HostedDomain: "corp.test"}, "")

	assert.Equal(t, "/", resp.Header.Get("Location"))
}

func TestSigninADeniedConsentLandsOnLoginWithAnError(t *testing.T) {
	env, fake := signinEnv(t)
	b := env.NewRedirectBrowser(t)
	authURL := startSignin(t, b, "")

	resp := b.Get(t, fake.Deny(t, authURL))

	assert.Equal(t, signin.ReasonDenied, refusalCode(t, resp, loginErrorPath))
	assert.Equal(t, http.StatusUnauthorized, meStatus(t, b))
}

func TestSigninABadStateIsRefused(t *testing.T) {
	env, fake := signinEnv(t)
	b := env.NewRedirectBrowser(t)
	authURL := startSignin(t, b, "")
	callback, err := url.Parse(fake.Authorize(t, authURL, newcomer))
	require.NoError(t, err)
	q := callback.Query()
	q.Set("state", "forged-state")
	callback.RawQuery = q.Encode()

	resp := b.Get(t, callback.RequestURI())

	assert.Equal(t, signin.ReasonInvalidState, refusalCode(t, resp, loginErrorPath))
	exists, err := env.DB.User.Query().Where(entuser.Email(newcomer.Email)).Exist(t.Context())
	require.NoError(t, err)
	assert.False(t, exists, "nothing was created")
}

func TestSigninACallbackFromAnotherBrowserIsRefused(t *testing.T) {
	env, fake := signinEnv(t)
	authURL := startSignin(t, env.NewRedirectBrowser(t), "")
	elsewhere := env.NewRedirectBrowser(t)

	resp := elsewhere.Get(t, fake.Authorize(t, authURL, newcomer))

	assert.Equal(t, signin.ReasonInvalidState, refusalCode(t, resp, loginErrorPath))
}

func TestSigninACallbackCannotBeReplayed(t *testing.T) {
	env, fake := signinEnv(t)
	b := env.NewRedirectBrowser(t)
	authURL := startSignin(t, b, "")
	callback := fake.Authorize(t, authURL, newcomer)
	b.Get(t, callback)

	resp := b.Get(t, callback)

	assert.Equal(t, signin.ReasonProviderError, refusalCode(t, resp, loginErrorPath), "the provider's code works once")
}

func TestSigninStartKeepsTheFlowInAHttpOnlyLaxCookieScopedToTheSigninPrefix(t *testing.T) {
	env, _ := signinEnv(t)
	b := env.NewRedirectBrowser(t)

	resp := b.Get(t, signinStartPath)

	cookies := resp.Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, "SIGNIN", cookies[0].Name)
	assert.True(t, cookies[0].HttpOnly)
	assert.Equal(t, http.SameSiteLaxMode, cookies[0].SameSite)
	assert.Equal(t, "/site/auth/signin", cookies[0].Path)
	assert.Positive(t, cookies[0].MaxAge)
}

func TestSigninReturnToARelativePathIsHonoured(t *testing.T) {
	env, fake := signinEnv(t)

	resp := signInWith(t, fake, env.NewRedirectBrowser(t), newcomer, "/w/abc/contacts?tab=1")

	assert.Equal(t, "/w/abc/contacts?tab=1", resp.Header.Get("Location"))
}

func TestSigninReturnToAnAbsoluteOrForeignUrlIsIgnored(t *testing.T) {
	for _, target := range []string{"https://evil.test/steal", "//evil.test", `/\evil.test`, "javascript:alert(1)"} {
		t.Run(target, func(t *testing.T) {
			env, fake := signinEnv(t)

			resp := signInWith(t, fake, env.NewRedirectBrowser(t), newcomer, target)

			assert.Equal(t, "/", resp.Header.Get("Location"))
		})
	}
}

func TestSigninAnUnknownProviderIsNotFoundAtStart(t *testing.T) {
	env, _ := signinEnv(t)

	resp := env.NewRedirectBrowser(t).Get(t, "/site/auth/signin/gitlab/start")

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestSigninACallbackForAProviderThatIsNotEnabledLandsOnLogin(t *testing.T) {
	env, _ := signinEnv(t)

	resp := env.NewRedirectBrowser(t).Get(t, "/site/auth/signin/gitlab/callback?code=x&state=y")

	assert.Equal(t, signin.ReasonProviderUnavailable, refusalCode(t, resp, loginErrorPath))
}

func TestSigninClosedSignupRefusesANewPersonButNotAKnownOne(t *testing.T) {
	env, fake := signinEnv(t, testhelper.WithConfig(func(c *config.Config) { c.SignupOpen = false }))

	refused := signInWith(t, fake, env.NewRedirectBrowser(t), newcomer, "")
	allowed := signInWith(t, fake, env.NewRedirectBrowser(t), mary, "")

	assert.Equal(t, signin.ReasonSignupClosed, refusalCode(t, refused, loginErrorPath))
	assert.Equal(t, "/", allowed.Header.Get("Location"))
}

// samIdentity is Sam, whose account has a Linked identity and an enrolled Second factor.
var samIdentity = testhelper.SigninIdentity{Subject: fixtures.SamGoogleIdentitySubject}

// challengeOf is the login challenge the callback put on the login page's address.
func challengeOf(t *testing.T, resp testhelper.Reply) string {
	t.Helper()
	require.Equal(t, http.StatusFound, resp.StatusCode)
	u, err := url.Parse(resp.Header.Get("Location"))
	require.NoError(t, err)
	require.Equal(t, loginErrorPath, u.Path)
	require.Empty(t, u.Query().Get("error"))
	challenge := u.Query().Get("challenge")
	require.NotEmpty(t, challenge)
	return challenge
}

func secondStepStatus(t *testing.T, env *testhelper.TestEnv, challenge, code string) *httptest.ResponseRecorder {
	t.Helper()
	return sendWithSession(t, env, http.MethodPost, secondStepPath, "",
		fmt.Sprintf(`{"challenge":%q,"code":%q}`, challenge, code))
}

// samTOTP is the authenticator app's current code for SecondFactorSam, on the env's clock.
func samTOTP(t *testing.T, clock *movableClock) string {
	t.Helper()
	c, err := totp.GenerateCode(fixtures.SecondFactorSamTotpSecret, clock.now())
	require.NoError(t, err)
	return c
}

func TestSigninAUserWithASecondFactorGetsAChallengeAndNoSession(t *testing.T) {
	env, fake := signinEnv(t)
	b := env.NewRedirectBrowser(t)

	resp := signInWith(t, fake, b, samIdentity, "")

	challengeOf(t, resp)
	assert.Nil(t, sessionCookie(&http.Response{Header: resp.Header}), "no session cookie before the code")
	assert.Equal(t, http.StatusUnauthorized, meStatus(t, b))
}

func TestSigninTheChallengeIsFinishedWithATOTPCode(t *testing.T) {
	env, fake, clock := signinEnvOnClock(t)
	challenge := challengeOf(t, signInWith(t, fake, env.NewRedirectBrowser(t), samIdentity, ""))

	rec := secondStepStatus(t, env, challenge, samTOTP(t, clock))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	cookie := sessionCookie(rec.Result())
	require.NotNil(t, cookie)
	assert.Equal(t, http.StatusOK, workspacesStatus(t, env, cookie.Value))
}

func TestSigninTheChallengeKeepsTheReturnAddress(t *testing.T) {
	env, fake := signinEnv(t)

	resp := signInWith(t, fake, env.NewRedirectBrowser(t), samIdentity, "/w/abc/contacts")

	challengeOf(t, resp)
	u, err := url.Parse(resp.Header.Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "/w/abc/contacts", u.Query().Get("redirect"))
}

func TestSigninTheChallengeIsFinishedWithARecoveryCode(t *testing.T) {
	env, fake := signinEnv(t)
	challenge := challengeOf(t, signInWith(t, fake, env.NewRedirectBrowser(t), samIdentity, ""))

	rec := secondStepStatus(t, env, challenge, fixtures.SecondFactorSamRecoveryCode)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.NotNil(t, sessionCookie(rec.Result()))
}

func TestSigninWrongCodesAfterAProviderLoginAreThrottled(t *testing.T) {
	env, fake, clock := signinEnvOnClock(t, testhelper.WithRateLimits(config.RateLimits{LoginFailures: 3}))
	challenge := challengeOf(t, signInWith(t, fake, env.NewRedirectBrowser(t), samIdentity, ""))
	for range 3 {
		require.Equal(t, http.StatusUnauthorized, secondStepStatus(t, env, challenge, "000000").Code)
	}

	rec := secondStepStatus(t, env, challenge, samTOTP(t, clock))

	assert.Equal(t, http.StatusTooManyRequests, rec.Code, "while the delay runs even a right code is refused")
}

func TestSigninAPasswordlessUserIsAnsweredLikeAWrongPassword(t *testing.T) {
	env, fake := signinEnv(t)
	signInWith(t, fake, env.NewRedirectBrowser(t), newcomer, "")

	passwordless := postLogin(t, env, newcomer.Email, "any password at all")
	unknown := postLogin(t, env, "nobody@gmail.com", "any password at all")

	assert.Equal(t, http.StatusUnauthorized, passwordless.Code)
	assert.Equal(t, unknown.Code, passwordless.Code)
	assert.Equal(t, unknown.Body.String(), passwordless.Body.String())
}

func TestSigninProvidersAreListedOnlyWhenConfigured(t *testing.T) {
	with, _ := signinEnv(t)
	without := testhelper.Setup(t)

	listed := with.NewRedirectBrowser(t).Get(t, "/site/auth/signin-providers")
	none := without.NewRedirectBrowser(t).Get(t, "/site/auth/signin-providers")

	assert.Equal(t, http.StatusOK, listed.StatusCode)
	assert.JSONEq(t, `{"items":[{"name":"google","displayName":"Google"}]}`, listed.Body)
	assert.JSONEq(t, `{"items":[]}`, none.Body)
}

func TestSigninStartIsNotFoundWhenNoProviderIsConfigured(t *testing.T) {
	env := testhelper.Setup(t)

	resp := env.NewRedirectBrowser(t).Get(t, signinStartPath)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}
