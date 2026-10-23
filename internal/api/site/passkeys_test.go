package site_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const (
	passkeysPath          = "/site/me/passkeys"
	passkeyBeginPath      = "/site/me/passkeys/registration"
	passkeyFinishPath     = "/site/me/passkeys/registration/finish"
	passkeyCeremonyCookie = "PASSKEY"
)

// passkeyCall is one request of a Passkey ceremony: the session cookie and, when set, the ceremony cookie.
func passkeyCall(t *testing.T, env *testhelper.TestEnv, method, path, session, ceremony, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(testhelper.RequestCookie("JWT", session))
	req.AddCookie(testhelper.RequestCookie(passkeyCeremonyCookie, ceremony))
	env.Server.ServeHTTP(rec, req)
	return rec
}

// ceremonyCookie is the value of the ceremony cookie a response sets.
func ceremonyCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	res := rec.Result()
	defer func() { require.NoError(t, res.Body.Close()) }()
	cookies := res.Cookies()
	for _, c := range cookies {
		if c.Name == passkeyCeremonyCookie {
			return c
		}
	}
	require.Fail(t, "no ceremony cookie")
	return nil
}

// begin starts a registration and returns the creation options and the ceremony cookie value.
func begin(t *testing.T, env *testhelper.TestEnv, session string) (options []byte, ceremony string) {
	t.Helper()
	rec := passkeyCall(t, env, http.MethodPost, passkeyBeginPath, session, "", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var res struct {
		Options json.RawMessage `json:"options"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	return res.Options, ceremonyCookie(t, rec).Value
}

// finishBody is the finish request: the proof, the name and the authenticator's response.
func finishBody(t *testing.T, password, name string, credential []byte) string {
	t.Helper()
	body := map[string]any{"name": name, "credential": json.RawMessage(credential)}
	if password != "" {
		body["currentPassword"] = password
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	return string(raw)
}

type passkeyItem struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	LastUsedAt *string `json:"lastUsedAt"`
}

type passkeyList struct {
	Items              []passkeyItem `json:"items"`
	Available          bool          `json:"available"`
	PasswordSet        bool          `json:"passwordSet"`
	IdentityProofFresh bool          `json:"identityProofFresh"`
}

func listPasskeys(t *testing.T, env *testhelper.TestEnv, session string) passkeyList {
	t.Helper()
	rec := passkeyCall(t, env, http.MethodGet, passkeysPath, session, "", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var list passkeyList
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	return list
}

func names(items []passkeyItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Name
	}
	return out
}

func patSession(t *testing.T, env *testhelper.TestEnv) string {
	t.Helper()
	return env.SiteToken(t, fixtures.PasskeyPatEmail, nil)
}

func TestAUserListsThePasskeysTheyHold(t *testing.T) {
	env := testhelper.Setup(t)

	list := listPasskeys(t, env, patSession(t, env))

	assert.Equal(t, []string{fixtures.PasskeyPatLaptopName}, names(list.Items))
	assert.True(t, list.PasswordSet)
	assert.True(t, list.Available)
}

func TestAUserWithAPasswordRegistersAPasskeyAndSeesItListed(t *testing.T) {
	env := testhelper.Setup(t)
	session := patSession(t, env)
	other := patSession(t, env)
	authenticator := env.NewAuthenticator(t)
	options, ceremony := begin(t, env, session)
	credential := authenticator.Attest(options)

	rec := passkeyCall(t, env, http.MethodPost, passkeyFinishPath, session, ceremony,
		finishBody(t, fixtures.PasskeyPatPassword, "Pat's phone", credential))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	fresh := reissued(t, rec)
	require.NotEmpty(t, fresh, "the acting session is reissued")
	list := listPasskeys(t, env, fresh)
	assert.Equal(t, []string{fixtures.PasskeyPatLaptopName, "Pat's phone"}, names(list.Items))
	assert.Equal(t, http.StatusUnauthorized, workspacesStatus(t, env, other), "another session ended")
}

func TestRegisteringAPasskeyMailsTheOwner(t *testing.T) {
	env := testhelper.Setup(t)
	session := patSession(t, env)
	options, ceremony := begin(t, env, session)
	credential := env.NewAuthenticator(t).Attest(options)

	rec := passkeyCall(t, env, http.MethodPost, passkeyFinishPath, session, ceremony,
		finishBody(t, fixtures.PasskeyPatPassword, "Pat's phone", credential))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	sent := env.SystemMail.Messages()
	require.Len(t, sent, 1)
	assert.Equal(t, fixtures.PasskeyPatEmail, sent[0].To)
	assert.Contains(t, sent[0].Subject, "passkey was added")
	assert.Contains(t, sent[0].Text, "Pat's phone")
}

func TestTheCreationOptionsAskForADiscoverableCredentialAndUserVerification(t *testing.T) {
	env := testhelper.Setup(t)

	options, _ := begin(t, env, patSession(t, env))

	var parsed struct {
		AuthenticatorSelection struct {
			ResidentKey      string `json:"residentKey"`
			UserVerification string `json:"userVerification"`
		} `json:"authenticatorSelection"`
		ExcludeCredentials []struct {
			ID string `json:"id"`
		} `json:"excludeCredentials"`
	}
	require.NoError(t, json.Unmarshal(options, &parsed))
	assert.Equal(t, "required", parsed.AuthenticatorSelection.ResidentKey)
	assert.Equal(t, "required", parsed.AuthenticatorSelection.UserVerification)
	require.Len(t, parsed.ExcludeCredentials, 1, "the Passkey the User holds is excluded")
	assert.Equal(t, fixtures.PasskeyPatLaptopCredentialID, parsed.ExcludeCredentials[0].ID)
}

func TestRegistrationIsRefusedWhenTheAuthenticatorCannotVerifyTheUser(t *testing.T) {
	env := testhelper.Setup(t)
	session := patSession(t, env)
	options, ceremony := begin(t, env, session)
	credential := env.NewAuthenticator(t, testhelper.WithoutUserVerification()).Attest(options)

	rec := passkeyCall(t, env, http.MethodPost, passkeyFinishPath, session, ceremony,
		finishBody(t, fixtures.PasskeyPatPassword, "A key without a PIN", credential))

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Equal(t, []string{fixtures.PasskeyPatLaptopName}, names(listPasskeys(t, env, session).Items))
}

func TestAReplayedFinishIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	session := patSession(t, env)
	options, ceremony := begin(t, env, session)
	credential := env.NewAuthenticator(t).Attest(options)
	body := finishBody(t, fixtures.PasskeyPatPassword, "Pat's phone", credential)
	first := passkeyCall(t, env, http.MethodPost, passkeyFinishPath, session, ceremony, body)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	fresh := reissued(t, first)

	replay := passkeyCall(t, env, http.MethodPost, passkeyFinishPath, fresh, ceremony, body)

	assert.Equal(t, http.StatusUnprocessableEntity, replay.Code)
	assert.Equal(t, []string{fixtures.PasskeyPatLaptopName, "Pat's phone"}, names(listPasskeys(t, env, fresh).Items))
}

func TestAnotherUserCannotSeeOrFinishAPasskeyCeremonyOfTheFirst(t *testing.T) {
	env := testhelper.Setup(t)
	pat := patSession(t, env)
	john := env.SiteToken(t, fixtures.OwnerJohnEmail, nil)
	options, ceremony := begin(t, env, pat)
	credential := env.NewAuthenticator(t).Attest(options)

	rec := passkeyCall(t, env, http.MethodPost, passkeyFinishPath, john, ceremony,
		finishBody(t, fixtures.OwnerJohnPassword, "John's phone", credential))

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, "the ceremony belongs to Pat")
	assert.Empty(t, listPasskeys(t, env, john).Items, "John sees none of Pat's Passkeys")
	assert.Equal(t, []string{fixtures.PasskeyPatLaptopName}, names(listPasskeys(t, env, pat).Items))
}

func TestAWrongPasswordRegistersNothing(t *testing.T) {
	env := testhelper.Setup(t)
	session := patSession(t, env)
	options, ceremony := begin(t, env, session)
	credential := env.NewAuthenticator(t).Attest(options)

	rec := passkeyCall(t, env, http.MethodPost, passkeyFinishPath, session, ceremony,
		finishBody(t, "not the password", "Pat's phone", credential))

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Equal(t, []string{fixtures.PasskeyPatLaptopName}, names(listPasskeys(t, env, session).Items))
	assert.Empty(t, env.SystemMail.Messages())
}

func TestFinishWithoutACeremonyCookieIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	session := patSession(t, env)
	options, _ := begin(t, env, session)
	credential := env.NewAuthenticator(t).Attest(options)

	rec := passkeyCall(t, env, http.MethodPost, passkeyFinishPath, session, "",
		finishBody(t, fixtures.PasskeyPatPassword, "Pat's phone", credential))

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func TestAPasskeyNeedsAName(t *testing.T) {
	env := testhelper.Setup(t)
	session := patSession(t, env)
	options, ceremony := begin(t, env, session)
	credential := env.NewAuthenticator(t).Attest(options)

	rec := passkeyCall(t, env, http.MethodPost, passkeyFinishPath, session, ceremony,
		finishBody(t, fixtures.PasskeyPatPassword, "  ", credential))

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Contains(t, rec.Body.String(), "name")
}

func TestPasskeyCallsNeedASession(t *testing.T) {
	env := testhelper.Setup(t)

	list := passkeyCall(t, env, http.MethodGet, passkeysPath, "", "", "")
	begun := passkeyCall(t, env, http.MethodPost, passkeyBeginPath, "", "", "")

	assert.Equal(t, http.StatusUnauthorized, list.Code)
	assert.Equal(t, http.StatusUnauthorized, begun.Code)
}

func TestAnInstanceServedOnAnIPAddressOffersNoPasskeys(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithConfig(func(c *config.Config) { c.AppURL = "http://127.0.0.1:3300" }))
	session := patSession(t, env)

	list := listPasskeys(t, env, session)
	begun := passkeyCall(t, env, http.MethodPost, passkeyBeginPath, session, "", "")

	assert.False(t, list.Available)
	assert.Equal(t, http.StatusNotFound, begun.Code)
}

func TestAUserWithNoPasswordRegistersAPasskeyAfterAFreshProviderLogin(t *testing.T) {
	r := newReauthEnv(t)
	session := r.signIn(t, newcomer)
	options, ceremony := begin(t, r.env, session)
	credential := r.env.NewAuthenticator(t).Attest(options)

	rec := passkeyCall(t, r.env, http.MethodPost, passkeyFinishPath, session, ceremony,
		finishBody(t, "", "Nina's laptop", credential))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	list := listPasskeys(t, r.env, reissued(t, rec))
	assert.Equal(t, []string{"Nina's laptop"}, names(list.Items))
	assert.False(t, list.PasswordSet)
}

func TestAnExpiredProviderLoginDoesNotProveAPasskeyRegistration(t *testing.T) {
	r := newReauthEnv(t)
	session := r.signIn(t, newcomer)
	options, ceremony := begin(t, r.env, session)
	credential := r.env.NewAuthenticator(t).Attest(options)
	r.clock.t = r.clock.t.Add(providerProofWindowPassed)

	rec := passkeyCall(t, r.env, http.MethodPost, passkeyFinishPath, session, ceremony,
		finishBody(t, "", "Nina's laptop", credential))

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "sign in again with your provider")
}

// providerProofWindowPassed is longer than the five minutes a provider login proves identity.
const providerProofWindowPassed = 6 * time.Minute
