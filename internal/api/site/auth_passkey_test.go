package site_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const (
	passkeyLoginBeginPath  = "/site/auth/passkey"
	passkeyLoginFinishPath = "/site/auth/passkey/finish"
)

// passkeyLoginCall is one request of a Passkey login: anonymous, with the ceremony cookie when set.
func passkeyLoginCall(t *testing.T, env *testhelper.TestEnv, path, ceremony, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: passkeyCeremonyCookie, Value: ceremony})
	env.Server.ServeHTTP(rec, req)
	return rec
}

// beginLogin starts a Passkey login and returns the request options and the ceremony cookie value.
func beginLogin(t *testing.T, env *testhelper.TestEnv) (options []byte, ceremony string) {
	t.Helper()
	rec := passkeyLoginCall(t, env, passkeyLoginBeginPath, "", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var res struct {
		Options json.RawMessage `json:"options"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	return res.Options, ceremonyCookie(t, rec).Value
}

func loginBodyOf(t *testing.T, credential []byte) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"credential": json.RawMessage(credential)})
	require.NoError(t, err)
	return string(raw)
}

// finishLogin posts the authenticator's response under the ceremony cookie.
func finishLogin(t *testing.T, env *testhelper.TestEnv, ceremony string, credential []byte) *httptest.ResponseRecorder {
	t.Helper()
	return passkeyLoginCall(t, env, passkeyLoginFinishPath, ceremony, loginBodyOf(t, credential))
}

// loginWithFixturePasskey runs a whole usernameless login of Pat with the fixture Passkey.
func loginWithFixturePasskey(t *testing.T, env *testhelper.TestEnv, opts ...testhelper.AuthenticatorOption) *httptest.ResponseRecorder {
	t.Helper()
	options, ceremony := beginLogin(t, env)
	return finishLogin(t, env, ceremony, env.FixturePasskey(t, opts...).Assert(options))
}

// methodOf is the method a session token records: read from the token's claims, as the
// Two-factor requirement reads it.
func methodOf(t *testing.T, token string) string {
	t.Helper()
	parts := strings.Split(token, ".")
	require.Len(t, parts, 3)
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	var claims struct {
		User struct {
			Attrs map[string]any `json:"attrs"`
		} `json:"user"`
	}
	require.NoError(t, json.Unmarshal(payload, &claims))
	method, _ := claims.User.Attrs["method"].(string)
	return method
}

func TestAUserLogsInWithAPasskeyWithoutAnEmailOrAPasswordOrASecondStep(t *testing.T) {
	env := testhelper.Setup(t)

	rec := loginWithFixturePasskey(t, env)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"outcome":"session"}`, rec.Body.String())
	cookie := sessionCookie(rec.Result())
	require.NotNil(t, cookie, "the login sets the session cookie")
	assert.Equal(t, http.StatusOK, workspacesStatus(t, env, cookie.Value), "the cookie authorizes /site")
}

func TestAPasskeyLoginStartsASessionWhoseMethodIsPasskey(t *testing.T) {
	env := testhelper.Setup(t)

	rec := loginWithFixturePasskey(t, env)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	cookie := sessionCookie(rec.Result())
	require.NotNil(t, cookie)
	assert.Equal(t, "passkey", methodOf(t, cookie.Value))
}

func TestAPasskeyLoginReplacesTheSecondFactorOfAUserWhoHasOne(t *testing.T) {
	env := testhelper.Setup(t)
	sam := env.SiteToken(t, fixtures.SecondFactorSamEmail, nil)
	authenticator := env.NewAuthenticator(t)
	creation, registration := begin(t, env, sam)
	added := passkeyCall(t, env, http.MethodPost, passkeyFinishPath, sam, registration,
		finishBody(t, fixtures.SecondFactorSamPassword, "Sam's laptop", authenticator.Attest(creation)))
	require.Equal(t, http.StatusOK, added.Code, added.Body.String())
	options, ceremony := beginLogin(t, env)

	rec := finishLogin(t, env, ceremony, authenticator.Assert(options))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"outcome":"session"}`, rec.Body.String(), "no challenge follows")
}

func TestThePasskeyLoginOptionsAskForUserVerificationAndNameNoCredential(t *testing.T) {
	env := testhelper.Setup(t)

	options, _ := beginLogin(t, env)

	var parsed struct {
		UserVerification string `json:"userVerification"`
		AllowCredentials []any  `json:"allowCredentials"`
	}
	require.NoError(t, json.Unmarshal(options, &parsed))
	assert.Equal(t, "required", parsed.UserVerification)
	assert.Empty(t, parsed.AllowCredentials, "the User is found by the credential, not named up front")
}

func TestAPasskeyLoginRecordsTheUseOnThePasskey(t *testing.T) {
	env := testhelper.Setup(t)
	require.Nil(t, listPasskeys(t, env, patSession(t, env)).Items[0].LastUsedAt)

	rec := loginWithFixturePasskey(t, env)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	list := listPasskeys(t, env, sessionCookie(rec.Result()).Value)
	assert.NotNil(t, list.Items[0].LastUsedAt)
}

func TestAnAssertionWithoutUserVerificationIsRefused(t *testing.T) {
	env := testhelper.Setup(t)

	rec := loginWithFixturePasskey(t, env, testhelper.WithoutUserVerification())

	assert.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
	assert.Nil(t, sessionCookie(rec.Result()))
}

func TestAReplayedAssertionIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	options, ceremony := beginLogin(t, env)
	assertion := env.FixturePasskey(t).Assert(options)
	first := finishLogin(t, env, ceremony, assertion)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())

	replay := finishLogin(t, env, ceremony, assertion)

	assert.Equal(t, http.StatusUnauthorized, replay.Code)
	assert.Nil(t, sessionCookie(replay.Result()))
}

func TestASecondAssertionOnASpentChallengeIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	options, ceremony := beginLogin(t, env)
	authenticator := env.FixturePasskey(t)
	first := finishLogin(t, env, ceremony, authenticator.Assert(options))
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())

	again := finishLogin(t, env, ceremony, authenticator.Assert(options))

	assert.Equal(t, http.StatusUnauthorized, again.Code, "a fresh, valid signature on a spent challenge")
}

func TestAnAssertionWithoutACeremonyCookieIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	options, _ := beginLogin(t, env)

	rec := finishLogin(t, env, "", env.FixturePasskey(t).Assert(options))

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAnAssertionForAnotherChallengeIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	options, _ := beginLogin(t, env)
	_, otherCeremony := beginLogin(t, env)

	rec := finishLogin(t, env, otherCeremony, env.FixturePasskey(t).Assert(options))

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestARegistrationCeremonyCookieDoesNotFinishALogin(t *testing.T) {
	env := testhelper.Setup(t)
	options, _ := beginLogin(t, env)
	_, registration := begin(t, env, patSession(t, env))

	rec := finishLogin(t, env, registration, env.FixturePasskey(t).Assert(options))

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAnUnknownCredentialGetsTheAnswerOfAnyOtherFailureAndMovesNoCounter(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithRateLimits(config.RateLimits{LoginFailures: 2}))
	stranger := env.NewAuthenticator(t)
	creation, _ := begin(t, env, patSession(t, env))
	stranger.Attest(creation)
	unverified := loginWithFixturePasskey(t, env, testhelper.WithoutUserVerification())
	require.Equal(t, http.StatusUnauthorized, unverified.Code)
	options, ceremony := beginLogin(t, env)

	unknown := finishLogin(t, env, ceremony, stranger.Assert(options))

	assert.Equal(t, unverified.Code, unknown.Code)
	assert.Equal(t, unverified.Body.String(), unknown.Body.String())
	for range 5 {
		options, ceremony := beginLogin(t, env)
		require.Equal(t, http.StatusUnauthorized, finishLogin(t, env, ceremony, stranger.Assert(options)).Code)
	}
	assert.Equal(t, http.StatusOK, loginStatus(t, env, fixtures.PasskeyPatEmail, fixtures.PasskeyPatPassword), "no failure was counted against anyone")
}

func TestAPasskeyThatFailsVerificationCountsNoFailure(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithRateLimits(config.RateLimits{LoginFailures: 2}))
	for range 5 {
		rec := loginWithFixturePasskey(t, env, testhelper.WithoutUserVerification())
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	}

	assert.Equal(t, http.StatusOK, loginStatus(t, env, fixtures.PasskeyPatEmail, fixtures.PasskeyPatPassword))
}

func TestTheLoginThrottleOfTheUserFoundStopsAPasskeyLogin(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithRateLimits(config.RateLimits{LoginFailures: 2}))
	for range 2 {
		require.Equal(t, http.StatusUnauthorized, loginStatus(t, env, fixtures.PasskeyPatEmail, "wrong"))
	}

	rec := loginWithFixturePasskey(t, env)

	assert.Equal(t, http.StatusTooManyRequests, rec.Code, "even a right assertion waits for the delay")
	assert.Nil(t, sessionCookie(rec.Result()))
}

func TestTheVerificationGateStopsAPasskeyLoginLikeAnyOtherFailure(t *testing.T) {
	env := testhelper.Setup(t)
	refused := loginWithFixturePasskey(t, env, testhelper.WithoutUserVerification())
	require.Equal(t, http.StatusUnauthorized, refused.Code)
	options, ceremony := beginLogin(t, env)

	rec := finishLogin(t, env, ceremony, env.FixtureUnverifiedPasskey(t).Assert(options))

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, refused.Body.String(), rec.Body.String(), "the gate does not say why")
	assert.Nil(t, sessionCookie(rec.Result()))
}

func TestAnInstanceServedOnAnIPAddressOffersNoPasskeyLogin(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithConfig(func(c *config.Config) { c.AppURL = "http://127.0.0.1:3300" }))

	rec := passkeyLoginCall(t, env, passkeyLoginBeginPath, "", "")

	assert.Equal(t, http.StatusNotFound, rec.Code)
}
