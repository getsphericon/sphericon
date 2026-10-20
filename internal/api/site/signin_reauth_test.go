package site_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const (
	enrollmentPath = "/site/me/second-factor/enrollment"
	confirmPath    = "/site/me/second-factor/enrollment/confirm"
	regeneratePath = "/site/me/second-factor/recovery-codes"
	disablePath    = "/site/me/second-factor/disable"
	statusPath     = "/site/me/second-factor"
)

// reauthEnv is a Sign-in provider env on a clock the test moves.
type reauthEnv struct {
	env   *testhelper.TestEnv
	fake  *testhelper.FakeSignin
	clock *movableClock
}

func newReauthEnv(t *testing.T) *reauthEnv {
	t.Helper()
	c := &movableClock{t: time.Now()}
	env, fake := signinEnv(t, testhelper.WithClock(c.now))
	return &reauthEnv{env: env, fake: fake, clock: c}
}

// signIn is a provider sign-in of who, returning the session cookie the callback set.
func (r *reauthEnv) signIn(t *testing.T, who testhelper.SigninIdentity) string {
	t.Helper()
	resp := signInWith(t, r.env, r.fake, r.env.NewRedirectBrowser(t), who, "")
	cookie := sessionCookie(&http.Response{Header: resp.Header})
	require.NotNil(t, cookie, "the callback started a session")
	return cookie.Value
}

// call sends a request under the session and returns the answer and the reissued session.
func (r *reauthEnv) call(t *testing.T, method, path, session, body string) (code int, next string, raw string) {
	t.Helper()
	rec := sendWithSession(t, r.env, method, path, session, body)
	return rec.Code, reissued(t, rec), rec.Body.String()
}

func (r *reauthEnv) totp(t *testing.T, secret string) string {
	t.Helper()
	c, err := totp.GenerateCode(secret, r.clock.now())
	require.NoError(t, err)
	return c
}

// enroll enrolls the session's User with no password and returns the reissued session.
func (r *reauthEnv) enroll(t *testing.T, session string) (next string, recoveryCodes []string, secret string) {
	t.Helper()
	code, _, raw := r.call(t, http.MethodPost, enrollmentPath, session, `{}`)
	require.Equal(t, http.StatusOK, code, raw)
	var e enrollmentRes
	require.NoError(t, json.Unmarshal([]byte(raw), &e))
	code, next, raw = r.call(t, http.MethodPost, confirmPath, session, `{"code":"`+r.totp(t, e.Secret)+`"}`)
	require.Equal(t, http.StatusOK, code, raw)
	var res recoveryCodesRes
	require.NoError(t, json.Unmarshal([]byte(raw), &res))
	return next, res.Codes, e.Secret
}

func TestReauthAPasswordlessUserWithAFreshProviderLoginEnrollsRegeneratesAndDisables(t *testing.T) {
	r := newReauthEnv(t)
	session := r.signIn(t, newcomer)

	session, _, secret := r.enroll(t, session)

	code, session, raw := r.call(t, http.MethodPost, regeneratePath, session, `{}`)
	require.Equal(t, http.StatusOK, code, raw)
	require.NotEmpty(t, session)
	r.clock.t = r.clock.t.Add(time.Minute) // a code works once: the next one is of the next step
	code, _, raw = r.call(t, http.MethodPost, disablePath, session, `{"code":"`+r.totp(t, secret)+`"}`)
	assert.Equal(t, http.StatusNoContent, code, raw)
}

func TestReauthAnExpiredProviderLoginIsRefusedUntilANewOne(t *testing.T) {
	r := newReauthEnv(t)
	session := r.signIn(t, newcomer)
	r.clock.t = r.clock.t.Add(time.Hour)

	code, _, raw := r.call(t, http.MethodPost, enrollmentPath, session, `{}`)

	assert.Equal(t, http.StatusForbidden, code)
	assert.Contains(t, raw, "sign in again with your provider")

	fresh := r.signIn(t, newcomer)
	code, _, raw = r.call(t, http.MethodPost, enrollmentPath, fresh, `{}`)
	assert.Equal(t, http.StatusOK, code, raw)
}

func TestReauthTheProofDoesNotOutliveItsWindowWhileEnrolling(t *testing.T) {
	r := newReauthEnv(t)
	session := r.signIn(t, newcomer)
	code, _, raw := r.call(t, http.MethodPost, enrollmentPath, session, `{}`)
	require.Equal(t, http.StatusOK, code, raw)
	var e enrollmentRes
	require.NoError(t, json.Unmarshal([]byte(raw), &e))
	r.clock.t = r.clock.t.Add(time.Hour)

	code, _, _ = r.call(t, http.MethodPost, confirmPath, session, `{"code":"`+r.totp(t, e.Secret)+`"}`)

	assert.Equal(t, http.StatusForbidden, code)
}

func TestReauthASessionThatDidNotComeFromAProviderIsNotAProof(t *testing.T) {
	r := newReauthEnv(t)
	r.signIn(t, newcomer)
	other := r.env.SiteToken(t, newcomer.Email, nil)

	code, _, _ := r.call(t, http.MethodPost, enrollmentPath, other, `{}`)

	assert.Equal(t, http.StatusForbidden, code)
}

func TestReauthAUserWithAPasswordStillProvesThePassword(t *testing.T) {
	r := newReauthEnv(t)
	maryNow := testhelper.SigninIdentity{Subject: fixtures.MaryGoogleIdentitySubject}
	session := r.signIn(t, maryNow)

	withoutPassword, _, _ := r.call(t, http.MethodPost, enrollmentPath, session, `{}`)
	withPassword, _, raw := r.call(t, http.MethodPost, enrollmentPath, session, `{"currentPassword":"`+fixtures.MemberMaryPassword+`"}`)

	assert.Equal(t, http.StatusForbidden, withoutPassword, "a fresh provider login does not replace a password that exists")
	assert.Equal(t, http.StatusOK, withPassword, raw)
}

func TestReauthTheProofSurvivesTheChallengeOfASecondFactor(t *testing.T) {
	r := newReauthEnv(t)
	_, _, secret := r.enroll(t, r.signIn(t, newcomer))
	challenge := challengeOf(t, signInWith(t, r.env, r.fake, r.env.NewRedirectBrowser(t), newcomer, ""))
	r.clock.t = r.clock.t.Add(time.Minute) // the enrollment's code is spent: the next one is of the next step

	rec := secondStepStatus(t, r.env, challenge, r.totp(t, secret))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	session := sessionCookie(rec.Result()).Value

	code, _, raw := r.call(t, http.MethodPost, regeneratePath, session, `{}`)
	assert.Equal(t, http.StatusOK, code, raw)
}

func TestReauthTheStatusTellsWhetherAProviderLoginIsNeeded(t *testing.T) {
	r := newReauthEnv(t)
	session := r.signIn(t, newcomer)
	type status struct {
		PasswordSet        bool `json:"passwordSet"`
		ProviderLoginFresh bool `json:"providerLoginFresh"`
	}
	read := func(session string) status {
		code, _, raw := r.call(t, http.MethodGet, statusPath, session, "")
		require.Equal(t, http.StatusOK, code, raw)
		var s status
		require.NoError(t, json.Unmarshal([]byte(raw), &s))
		return s
	}

	assert.Equal(t, status{PasswordSet: false, ProviderLoginFresh: true}, read(session))
	r.clock.t = r.clock.t.Add(time.Hour)
	assert.Equal(t, status{PasswordSet: false, ProviderLoginFresh: false}, read(session))

	john := r.env.SiteToken(t, fixtures.OwnerJohnEmail, nil)
	assert.Equal(t, status{PasswordSet: true, ProviderLoginFresh: false}, read(john))
}

func TestReauthLinkingANewProviderAccountDoesNotRefreshTheProof(t *testing.T) {
	r := newReauthEnv(t)
	b := r.env.NewRedirectBrowser(t)
	signInWith(t, r.env, r.fake, b, newcomer, "")
	r.clock.t = r.clock.t.Add(time.Hour) // the login's proof has expired
	attacker := testhelper.SigninIdentity{Subject: "google-subject-of-the-attacker", Email: "attacker@gmail.test", EmailVerified: true}

	resp := link(t, r.fake, b, attacker)

	cookie := sessionCookie(&http.Response{Header: resp.Header})
	require.NotNil(t, cookie, "the acting session is reissued")
	code, _, _ := r.call(t, http.MethodPost, enrollmentPath, cookie.Value, `{}`)
	assert.Equal(t, http.StatusForbidden, code, "a provider account added now is not a proof of the person")
}

func TestReauthLinkingAnAlreadyLinkedProviderAccountRefreshesTheProof(t *testing.T) {
	r := newReauthEnv(t)
	b := r.env.NewRedirectBrowser(t)
	signInWith(t, r.env, r.fake, b, newcomer, "")
	r.clock.t = r.clock.t.Add(time.Hour)

	resp := link(t, r.fake, b, newcomer)

	cookie := sessionCookie(&http.Response{Header: resp.Header})
	require.NotNil(t, cookie)
	code, _, raw := r.call(t, http.MethodPost, enrollmentPath, cookie.Value, `{}`)
	assert.Equal(t, http.StatusOK, code, raw)
}
