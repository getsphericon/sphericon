package site_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	gptoken "github.com/go-pkgz/auth/v2/token"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiauth "github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const (
	passkeyCheckBeginPath  = "/site/me/passkeys/check"
	passkeyCheckFinishPath = "/site/me/passkeys/check/finish"
	startEnrollmentPath    = "/site/me/second-factor/enrollment"
)

func pegSession(t *testing.T, env *testhelper.TestEnv) string {
	t.Helper()
	return env.SiteToken(t, fixtures.PasskeyOnlyPegEmail, nil)
}

// beginCheck starts a Passkey check and returns the request options and the ceremony cookie value.
func beginCheck(t *testing.T, env *testhelper.TestEnv, session string) (options []byte, ceremony string) {
	t.Helper()
	rec := passkeyCall(t, env, http.MethodPost, passkeyCheckBeginPath, session, "", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var res struct {
		Options json.RawMessage `json:"options"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	return res.Options, ceremonyCookie(t, rec).Value
}

func finishCheck(t *testing.T, env *testhelper.TestEnv, session, ceremony string, credential []byte) *httptest.ResponseRecorder {
	t.Helper()
	return passkeyCall(t, env, http.MethodPost, passkeyCheckFinishPath, session, ceremony, loginBodyOf(t, credential))
}

// checkWithPegPasskey runs a whole fresh Passkey check of Peg and returns the session it reissues.
func checkWithPegPasskey(t *testing.T, env *testhelper.TestEnv, session string) string {
	t.Helper()
	options, ceremony := beginCheck(t, env, session)
	rec := finishCheck(t, env, session, ceremony, env.FixturePegPasskey(t).Assert(options))
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	fresh := reissued(t, rec)
	require.NotEmpty(t, fresh, "the session is reissued with the check")
	return fresh
}

func TestAUserWithNoPasswordIsRefusedAProvedStepWithoutAFreshCheck(t *testing.T) {
	env := testhelper.Setup(t)

	rec := passkeyCall(t, env, http.MethodPost, startEnrollmentPath, pegSession(t, env), "", `{}`)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "Passkey")
}

func TestAFreshPasskeyCheckLetsAUserWithNoPasswordEnrollASecondFactor(t *testing.T) {
	env := testhelper.Setup(t)
	session := checkWithPegPasskey(t, env, pegSession(t, env))

	rec := passkeyCall(t, env, http.MethodPost, startEnrollmentPath, session, "", `{}`)

	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func TestAFreshPasskeyCheckLetsAUserWithNoPasswordAddAPasskey(t *testing.T) {
	env := testhelper.Setup(t)
	session := checkWithPegPasskey(t, env, pegSession(t, env))
	options, ceremony := begin(t, env, session)
	credential := env.NewAuthenticator(t).Attest(options)

	rec := passkeyCall(t, env, http.MethodPost, passkeyFinishPath, session, ceremony, finishBody(t, "", "Peg's phone", credential))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, []string{fixtures.PasskeyPegKeyName, "Peg's phone"}, names(listPasskeys(t, env, reissued(t, rec)).Items))
}

func TestAFreshPasskeyCheckLetsAUserWithNoPasswordRegenerateRecoveryCodes(t *testing.T) {
	env := testhelper.Setup(t)
	session := checkWithPegPasskey(t, env, pegSession(t, env))

	rec := passkeyCall(t, env, http.MethodPost, "/site/me/second-factor/recovery-codes", session, "", `{}`)

	assert.Equal(t, http.StatusConflict, rec.Code, "past the proof: there is no Second factor to regenerate codes for")
}

func TestThePasskeyCheckIsReflectedInTheList(t *testing.T) {
	env := testhelper.Setup(t)
	session := pegSession(t, env)
	assert.False(t, listPasskeys(t, env, session).IdentityProofFresh)

	fresh := checkWithPegPasskey(t, env, session)

	assert.True(t, listPasskeys(t, env, fresh).IdentityProofFresh)
}

func TestAnExpiredPasskeyCheckIsRefusedAndAsksForANewOne(t *testing.T) {
	env := testhelper.Setup(t)
	session := env.SiteToken(t, fixtures.PasskeyOnlyPegEmail, func(c *gptoken.Claims) {
		c.User.SetStrAttr(apiauth.ClaimPasskeyCheck, strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10))
	})

	rec := passkeyCall(t, env, http.MethodPost, startEnrollmentPath, session, "", `{}`)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "Passkey")
	assert.False(t, listPasskeys(t, env, session).IdentityProofFresh)
}

func TestAnotherUsersPasskeyIsNoCheckOfTheSessionUser(t *testing.T) {
	env := testhelper.Setup(t)
	session := pegSession(t, env)
	options, ceremony := beginCheck(t, env, session)
	var open map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(options, &open))
	delete(open, "allowCredentials") // a browser that ignores the list and offers Pat's Passkey
	options, err := json.Marshal(open)
	require.NoError(t, err)

	rec := finishCheck(t, env, session, ceremony, env.FixturePasskey(t).Assert(options))

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Empty(t, reissued(t, rec))
}

func TestAPasskeyCheckCannotBeReplayed(t *testing.T) {
	env := testhelper.Setup(t)
	session := pegSession(t, env)
	options, ceremony := beginCheck(t, env, session)
	assertion := env.FixturePegPasskey(t).Assert(options)
	require.Equal(t, http.StatusNoContent, finishCheck(t, env, session, ceremony, assertion).Code)

	replay := finishCheck(t, env, session, ceremony, assertion)

	assert.Equal(t, http.StatusForbidden, replay.Code)
}

func TestAPasskeyCheckWithoutUserVerificationIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	session := pegSession(t, env)
	options, ceremony := beginCheck(t, env, session)

	rec := finishCheck(t, env, session, ceremony, env.FixturePegPasskey(t, testhelper.WithoutUserVerification()).Assert(options))

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestACeremonyOfAnotherKindIsNoPasskeyCheck(t *testing.T) {
	env := testhelper.Setup(t)
	session := pegSession(t, env)
	options, ceremony := beginLogin(t, env)

	rec := finishCheck(t, env, session, ceremony, env.FixturePegPasskey(t).Assert(options))

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestAPasskeyCheckNeedsASessionAndAPasskey(t *testing.T) {
	env := testhelper.Setup(t)

	anonymous := passkeyCall(t, env, http.MethodPost, passkeyCheckBeginPath, "", "", "")
	noPasskey := passkeyCall(t, env, http.MethodPost, passkeyCheckBeginPath, env.SiteToken(t, fixtures.OwnerJohnEmail, nil), "", "")

	assert.Equal(t, http.StatusUnauthorized, anonymous.Code)
	assert.Equal(t, http.StatusNotFound, noPasskey.Code)
}

func TestAPasskeyLoginIsAFreshCheckToo(t *testing.T) {
	env := testhelper.Setup(t)
	options, ceremony := beginLogin(t, env)
	login := finishLogin(t, env, ceremony, env.FixturePegPasskey(t).Assert(options))
	require.Equal(t, http.StatusOK, login.Code, login.Body.String())

	rec := passkeyCall(t, env, http.MethodPost, startEnrollmentPath, sessionCookie(login.Result()).Value, "", `{}`)

	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func TestThePasswordPathIsUnchangedForAUserWithAPassword(t *testing.T) {
	env := testhelper.Setup(t)
	session := patSession(t, env)

	withoutPassword := passkeyCall(t, env, http.MethodPost, startEnrollmentPath, session, "", `{}`)
	withPassword := passkeyCall(t, env, http.MethodPost, startEnrollmentPath, session, "", `{"currentPassword":"`+fixtures.PasskeyPatPassword+`"}`)

	assert.Equal(t, http.StatusForbidden, withoutPassword.Code)
	assert.Equal(t, http.StatusOK, withPassword.Code, withPassword.Body.String())
}
