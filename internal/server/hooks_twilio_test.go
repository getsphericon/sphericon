package server_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The Auth Token of Acme's SMS Integration, spelled the way the fixture does.
const twilioAuthToken = "fixture-acme-twilio-auth-token"

// twilioStatusBody is a status callback of a message Twilio could not deliver.
const twilioStatusBody = "ErrorCode=30003&MessageSid=SM1342fe1b2c904d1ab04f0fc7a58abca9&MessageStatus=undelivered" +
	"&To=%2B14155552671&From=%2B15017250604&AccountSid=ACXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX&ApiVersion=2010-04-01"

// twilioSignature signs a form body for a URL as Twilio does ("Webhooks security").
func twilioSignature(t *testing.T, signedURL, body string) string {
	t.Helper()
	values, err := url.ParseQuery(body)
	require.NoError(t, err)
	content := []byte(signedURL)
	for _, name := range slices.Sorted(maps.Keys(values)) {
		for _, v := range slices.Sorted(slices.Values(values[name])) {
			content = append(content, name+v...)
		}
	}
	mac := hmac.New(sha1.New, []byte(twilioAuthToken))
	mac.Write(content)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// twilioPost posts a form body to target (a path and query of the server), signed for signedURL.
func twilioPost(t *testing.T, env *testhelper.TestEnv, target, signedURL, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Twilio-Signature", twilioSignature(t, signedURL, body))
	rec := httptest.NewRecorder()
	env.Server.ServeHTTP(rec, req)
	return rec
}

// Twilio signs the URL it posts to, which is the Integration's hook URL on the public address:
// the hook checks the signature against that URL with the query the request came with, however
// the request reached the server.
func TestATwilioCallbackIsCheckedAgainstTheHookURLItWasPostedTo(t *testing.T) {
	env := setupHooks(t)
	path := "/hooks/" + fixtures.IntegrationAcmeSmsIngestKey
	hookURL := env.AppURL + path

	plain := twilioPost(t, env, path, hookURL, twilioStatusBody)
	withQuery := twilioPost(t, env, path+"?region=ie1", hookURL+"?region=ie1", twilioStatusBody)

	assert.Equal(t, http.StatusOK, plain.Code)
	assert.Equal(t, http.StatusOK, withQuery.Code)
}

func TestATwilioCallbackSignedForAnotherURLIsRefused(t *testing.T) {
	env := setupHooks(t)
	path := "/hooks/" + fixtures.IntegrationAcmeSmsIngestKey
	hookURL := env.AppURL + path

	changedQuery := twilioPost(t, env, path+"?region=us1", hookURL+"?region=ie1", twilioStatusBody)
	droppedQuery := twilioPost(t, env, path, hookURL+"?region=ie1", twilioStatusBody)
	anotherHost := twilioPost(t, env, path, "https://attacker.example"+path, twilioStatusBody)

	assert.Equal(t, http.StatusForbidden, changedQuery.Code)
	assert.Equal(t, http.StatusForbidden, droppedQuery.Code)
	assert.Equal(t, http.StatusForbidden, anotherHost.Code)
}

// A Twilio account connected through the app has its callbacks signed with its Auth Token: the hook
// takes a callback so signed, and refuses and logs one signed with anything else, so nobody can
// forge a STOP, a delivery report or a reply.
func TestATwilioCallbackOfAConnectedAccountIsTakenAndAForgedOneRefusedAndLogged(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	env := setupHooks(t)
	sms := env.ConnectTwilio(t)
	status := url.Values{
		"MessageSid": {"SM0123"}, "MessageStatus": {"delivered"}, "To": {"+14155552671"},
		"From": {testhelper.TwilioNumber}, "AccountSid": {sms.Account.SID},
	}

	signed := env.PostTwilioCallback(t, sms.Integration.HookUrl, sms.Account.AuthToken, status)
	require.Equal(t, http.StatusOK, signed.Code, signed.Body.String())
	assert.NotContains(t, logs.String(), "not authentic")

	forged := env.PostTwilioCallback(t, sms.Integration.HookUrl, "notTheAuthToken", status)
	assert.Equal(t, http.StatusForbidden, forged.Code)
	assert.Contains(t, logs.String(), `"level":"WARN"`)
	assert.Contains(t, logs.String(), "not authentic")
	assert.Contains(t, logs.String(), `"preset":"twilio"`)
}
