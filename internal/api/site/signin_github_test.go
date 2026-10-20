package site_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	entuser "github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/ent/useridentity"
	"github.com/getsphericon/sphericon/internal/signin"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const signinGitHubStartPath = "/site/auth/signin/github/start"

// octo is a GitHub account whose primary email GitHub verified.
var octo = testhelper.SigninIdentity{Subject: "583231", Login: "octo", Name: "Octo Cat", Email: "octo@corp.test", EmailVerified: true}

func githubEnv(t *testing.T, opts ...testhelper.Option) (*testhelper.TestEnv, *testhelper.FakeSignin) {
	t.Helper()
	fake := testhelper.NewFakeSignin(t)
	return testhelper.Setup(t, append([]testhelper.Option{testhelper.WithSigninGitHub(fake)}, opts...)...), fake
}

func signInWithGitHub(t *testing.T, fake *testhelper.FakeSignin, b *testhelper.RedirectBrowser, who testhelper.SigninIdentity) testhelper.Reply {
	t.Helper()
	resp := b.Get(t, signinGitHubStartPath)
	require.Equal(t, http.StatusFound, resp.StatusCode)
	return b.Get(t, fake.Authorize(t, resp.Header.Get("Location"), who))
}

func TestSigninGitHubANewPersonIsSignedUpFromTheirPrimaryVerifiedEmail(t *testing.T) {
	env, fake := githubEnv(t)
	b := env.NewRedirectBrowser(t)
	before := len(env.SystemMail.Messages())

	resp := signInWithGitHub(t, fake, b, octo)

	assert.Equal(t, "/", resp.Header.Get("Location"))
	assert.Equal(t, http.StatusOK, meStatus(t, b))
	u, err := env.DB.User.Query().Where(entuser.Email(octo.Email)).Only(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, u.EmailVerifiedAt)
	assert.Equal(t, "Octo Cat", u.Name)
	assert.Empty(t, u.PasswordHash)
	assert.Len(t, env.SystemMail.Messages(), before, "no mail is sent")
	identity, err := env.DB.UserIdentity.Query().Where(useridentity.Provider("github"), useridentity.Subject(octo.Subject)).Only(t.Context())
	require.NoError(t, err)
	assert.Equal(t, u.ID, identity.UserID)
}

func TestSigninGitHubAnAccountWithoutANameUsesItsLogin(t *testing.T) {
	env, fake := githubEnv(t)
	who := octo
	who.Name = ""

	signInWithGitHub(t, fake, env.NewRedirectBrowser(t), who)

	u, err := env.DB.User.Query().Where(entuser.Email(octo.Email)).Only(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "octo", u.Name)
}

func TestSigninGitHubTheSamePersonSignsInAgainBySubject(t *testing.T) {
	env, fake := githubEnv(t)
	signInWithGitHub(t, fake, env.NewRedirectBrowser(t), octo)
	moved := octo
	moved.Email = "octo.new@corp.test"
	again := env.NewRedirectBrowser(t)

	resp := signInWithGitHub(t, fake, again, moved)

	assert.Equal(t, "/", resp.Header.Get("Location"))
	assert.Equal(t, http.StatusOK, meStatus(t, again))
	exists, err := env.DB.User.Query().Where(entuser.Email(moved.Email)).Exist(t.Context())
	require.NoError(t, err)
	assert.False(t, exists, "no second account was made for the new address")
}

func TestSigninGitHubRefusesWhenThereIsNoVerifiedPrimaryEmail(t *testing.T) {
	cases := map[string]testhelper.SigninIdentity{
		"an unverified primary email": {Subject: "1", Login: "a", Email: "a@corp.test", EmailVerified: false},
		"only a verified secondary email": {
			Subject: "2", Login: "b", Email: "b@corp.test", EmailVerified: false,
			OtherEmails: []testhelper.SigninEmail{{Email: "b2@corp.test", Verified: true}},
		},
		"no email at all": {Subject: "3", Login: "c"},
	}
	for name, who := range cases {
		t.Run(name, func(t *testing.T) {
			env, fake := githubEnv(t)
			b := env.NewRedirectBrowser(t)

			resp := signInWithGitHub(t, fake, b, who)

			assert.Equal(t, signin.ReasonEmailUnverified, refusalCode(t, resp, loginErrorPath))
			assert.Equal(t, http.StatusUnauthorized, meStatus(t, b))
		})
	}
}

func TestSigninGitHubTrustsAnyDomainUnlikeGoogle(t *testing.T) {
	env, fake := githubEnv(t)

	resp := signInWithGitHub(t, fake, env.NewRedirectBrowser(t), octo)

	assert.Equal(t, "/", resp.Header.Get("Location"), "a verified non-Gmail address with no hd is accepted from GitHub")
}

func TestSigninGitHubADeniedConsentLandsOnLoginWithAnError(t *testing.T) {
	env, fake := githubEnv(t)
	b := env.NewRedirectBrowser(t)
	resp := b.Get(t, signinGitHubStartPath)

	denied := b.Get(t, fake.Deny(t, resp.Header.Get("Location")))

	assert.Equal(t, signin.ReasonDenied, refusalCode(t, denied, loginErrorPath))
}

func TestSigninGitHubClosedSignupRefusesANewPerson(t *testing.T) {
	env, fake := githubEnv(t, testhelper.WithConfig(func(c *config.Config) { c.SignupOpen = false }))

	resp := signInWithGitHub(t, fake, env.NewRedirectBrowser(t), octo)

	assert.Equal(t, signin.ReasonSignupClosed, refusalCode(t, resp, loginErrorPath))
}

func TestSigninProvidersListsBothWhenBothAreConfigured(t *testing.T) {
	fake := testhelper.NewFakeSignin(t)
	env := testhelper.Setup(t, testhelper.WithSignin(fake), testhelper.WithSigninGitHub(fake))

	listed := env.NewRedirectBrowser(t).Get(t, "/site/auth/signin-providers")

	assert.JSONEq(t, `{"items":[{"name":"google","displayName":"Google"},{"name":"github","displayName":"GitHub"}]}`, listed.Body)
}
