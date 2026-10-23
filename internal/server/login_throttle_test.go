package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent/authattempt"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const (
	loginPath     = "/site/auth/login"
	loginFailures = 5
	// An address no account has.
	unknownLoginEmail = "nobody@nowhere.test"
)

// frozenClock is the injected `now` of the account attempt module.
type frozenClock struct{ t time.Time }

func (c *frozenClock) now() time.Time { return c.t }

func loginEnv(t *testing.T, ipLimit int) (*testhelper.TestEnv, *frozenClock) {
	t.Helper()
	c := &frozenClock{t: time.Now()}
	env := testhelper.Setup(t,
		testhelper.WithClock(c.now),
		testhelper.WithRateLimits(config.RateLimits{LoginFailures: loginFailures, LoginIP: ipLimit}))
	return env, c
}

func login(t *testing.T, env *testhelper.TestEnv, user, password string) *httptest.ResponseRecorder {
	t.Helper()
	return postJSON(t, env, loginPath, fmt.Sprintf(`{"email":%q,"password":%q}`, user, password), nil)
}

func failLogins(t *testing.T, env *testhelper.TestEnv, user string, times int) {
	t.Helper()
	for i := range times {
		rec := login(t, env, user, "wrong-password")
		require.Equal(t, http.StatusUnauthorized, rec.Code, "failure %d", i+1)
	}
}

func TestLoginAnswers429AfterTheConfiguredFailuresEvenWithTheRightPassword(t *testing.T) {
	env, _ := loginEnv(t, 0)
	failLogins(t, env, fixtures.OwnerJohnEmail, loginFailures)

	for name, password := range map[string]string{"wrong": "wrong-password", "correct": fixtures.OwnerJohnPassword} {
		rec := login(t, env, fixtures.OwnerJohnEmail, password)
		require.Equal(t, http.StatusTooManyRequests, rec.Code, name)
		assert.Equal(t, "1", rec.Header().Get("Retry-After"), name)
		assert.Equal(t, "0", rec.Header().Get("X-Ratelimit-Remaining"))
		assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
		var body struct {
			Status     int `json:"status"`
			RetryAfter int `json:"retryAfter"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, 429, body.Status)
		assert.Equal(t, 1, body.RetryAfter)
		assert.Empty(t, jwtOf(rec), "a correct password during the delay must not log in")
	}
}

func TestLoginWorksAgainOnceTheDelayHasPassedAndResetsTheCounter(t *testing.T) {
	env, clock := loginEnv(t, 0)
	failLogins(t, env, fixtures.OwnerJohnEmail, loginFailures)
	clock.t = clock.t.Add(2 * time.Second)

	rec := login(t, env, fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.NotEmpty(t, jwtOf(rec))

	n, err := env.DB.AuthAttempt.Query().Where(authattempt.Email(fixtures.OwnerJohnEmail)).Count(t.Context())
	require.NoError(t, err)
	assert.Zero(t, n, "a success resets the counter")
}

func TestASuccessBeforeTheThresholdResetsTheCounter(t *testing.T) {
	env, _ := loginEnv(t, 0)
	failLogins(t, env, fixtures.OwnerJohnEmail, loginFailures-1)
	require.Equal(t, http.StatusOK, login(t, env, fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword).Code)
	failLogins(t, env, fixtures.OwnerJohnEmail, loginFailures-1)
	assert.Equal(t, http.StatusOK, login(t, env, fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword).Code)
}

func TestUnknownEmailsThrottleLikeKnownOnesAndLeaveRows(t *testing.T) {
	env, _ := loginEnv(t, 0)
	failLogins(t, env, unknownLoginEmail, loginFailures)
	assert.Equal(t, http.StatusTooManyRequests, login(t, env, unknownLoginEmail, "whatever").Code)

	row, err := env.DB.AuthAttempt.Query().Where(authattempt.Email(unknownLoginEmail)).Only(t.Context())
	require.NoError(t, err)
	assert.Equal(t, loginFailures, row.Failures)

	// A counter deep into its delay (fixture) answers 429 straight away.
	assert.Equal(t, http.StatusTooManyRequests, login(t, env, fixtures.GhostLoginAttemptEmail, "whatever").Code)
}

func TestLoginIsLimitedPerIPAcrossAccounts(t *testing.T) {
	const ipLimit = 3
	env, _ := loginEnv(t, ipLimit)
	for i := range ipLimit {
		rec := login(t, env, fmt.Sprintf("spray%d@nowhere.test", i), "x")
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	}
	rec := login(t, env, "spray-last@nowhere.test", "x")
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))

	other := postJSON(t, env, loginPath, `{"email":"a@nowhere.test","password":"x"}`, map[string]string{"X-Forwarded-For": "203.0.113.9"})
	assert.Equal(t, http.StatusUnauthorized, other.Code, "another address has its own budget")
}

func TestDisabledLoginLimitsNeverThrottle(t *testing.T) {
	env := testhelper.Setup(t)
	failLogins(t, env, fixtures.OwnerJohnEmail, loginFailures+3)
	assert.Equal(t, http.StatusOK, login(t, env, fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword).Code)
}

func jwtOf(rec *httptest.ResponseRecorder) string {
	for _, c := range rec.Result().Cookies() {
		if c.Name == "JWT" {
			return c.Value
		}
	}
	return ""
}

// samChallenge passes Sam's password step (he has a Second factor) and returns the
// challenge.
func samChallenge(t *testing.T, env *testhelper.TestEnv) string {
	t.Helper()
	rec := login(t, env, fixtures.SecondFactorSamEmail, fixtures.SecondFactorSamPassword)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var res struct {
		Challenge string `json:"challenge"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	require.NotEmpty(t, res.Challenge)
	return res.Challenge
}

func secondStep(t *testing.T, env *testhelper.TestEnv, challenge, code string) *httptest.ResponseRecorder {
	t.Helper()
	return postJSON(t, env, "/site/auth/second-factor", fmt.Sprintf(`{"challenge":%q,"code":%q}`, challenge, code), nil)
}

func TestWrongSecondStepCodesFeedTheLoginThrottle(t *testing.T) {
	env, clock := loginEnv(t, 0)
	challenge := samChallenge(t, env)
	for i := range loginFailures {
		require.Equal(t, http.StatusUnauthorized, secondStep(t, env, challenge, "000000").Code, "failure %d", i+1)
	}

	good, err := totp.GenerateCode(fixtures.SecondFactorSamTotpSecret, clock.now())
	require.NoError(t, err)
	rec := secondStep(t, env, challenge, good)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code, "a correct code during the delay must not log in")
	assert.Empty(t, jwtOf(rec))
	assert.Equal(t, http.StatusTooManyRequests,
		login(t, env, fixtures.SecondFactorSamEmail, fixtures.SecondFactorSamPassword).Code, "one counter for both steps")
}

// Knowing the password must not buy a fresh round of code guesses: the password
// step does not reset the counter of a User with a Second factor.
func TestThePasswordStepDoesNotResetSecondStepFailures(t *testing.T) {
	env, _ := loginEnv(t, 0)
	for i := range loginFailures - 1 {
		require.Equal(t, http.StatusUnauthorized, secondStep(t, env, samChallenge(t, env), "000000").Code, "failure %d", i+1)
	}
	require.Equal(t, http.StatusUnauthorized, secondStep(t, env, samChallenge(t, env), "000000").Code)
	assert.Equal(t, http.StatusTooManyRequests,
		login(t, env, fixtures.SecondFactorSamEmail, fixtures.SecondFactorSamPassword).Code)
}

func TestASuccessfulSecondStepResetsTheCounter(t *testing.T) {
	env, clock := loginEnv(t, 0)
	challenge := samChallenge(t, env)
	for range loginFailures - 1 {
		require.Equal(t, http.StatusUnauthorized, secondStep(t, env, challenge, "000000").Code)
	}
	good, err := totp.GenerateCode(fixtures.SecondFactorSamTotpSecret, clock.now())
	require.NoError(t, err)
	rec := secondStep(t, env, challenge, good)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.NotEmpty(t, jwtOf(rec))

	n, err := env.DB.AuthAttempt.Query().Where(authattempt.Email(fixtures.SecondFactorSamEmail)).Count(t.Context())
	require.NoError(t, err)
	assert.Zero(t, n)
}

func TestTheSecondStepSharesThePerIPLoginCap(t *testing.T) {
	const ipLimit = 2
	env, _ := loginEnv(t, ipLimit)
	for range ipLimit {
		require.Equal(t, http.StatusUnauthorized, secondStep(t, env, "forged", "000000").Code)
	}
	assert.Equal(t, http.StatusTooManyRequests, secondStep(t, env, "forged", "000000").Code)
}

// A session holder proving the password to manage the Second factor guesses the same
// password as a login does: wrong ones feed the account's Login throttle, and while
// its delay runs even the right one answers 429.
func TestSecondFactorPasswordChecksFeedTheLoginThrottle(t *testing.T) {
	env, _ := loginEnv(t, 0)
	cookie := map[string]string{"Cookie": "JWT=" + env.SiteToken(t, fixtures.SecondFactorSamEmail, nil)}
	regenerate := func(password string) int {
		return postJSON(t, env, "/site/me/second-factor/recovery-codes",
			fmt.Sprintf(`{"currentPassword":%q}`, password), cookie).Code
	}
	for i := range loginFailures {
		require.Equal(t, http.StatusForbidden, regenerate("wrong-password"), "failure %d", i+1)
	}

	assert.Equal(t, http.StatusTooManyRequests, regenerate(fixtures.SecondFactorSamPassword))
	for path, body := range map[string]string{
		"/site/me/second-factor/disable":    fmt.Sprintf(`{"currentPassword":%q,"code":%q}`, fixtures.SecondFactorSamPassword, fixtures.SecondFactorSamRecoveryCode),
		"/site/me/second-factor/enrollment": fmt.Sprintf(`{"currentPassword":%q}`, fixtures.SecondFactorSamPassword),
	} {
		assert.Equal(t, http.StatusTooManyRequests, postJSON(t, env, path, body, cookie).Code, path)
	}
	assert.Equal(t, http.StatusTooManyRequests,
		login(t, env, fixtures.SecondFactorSamEmail, fixtures.SecondFactorSamPassword).Code, "one counter with the login")
}
