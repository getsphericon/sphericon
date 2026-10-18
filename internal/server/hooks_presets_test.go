package server_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The built-in provider Presets over HTTP, on the committed Acme Integrations that use them
// (fixtures/integrations.yml). Each case posts a provider's payload to the Integration's hook
// URL and asserts what it left behind: the response, the outbox, Suppression.

// The credentials of the Postmark and Mailgun Integrations, spelled the way the fixtures do.
const (
	postmarkUser   = "postmark-user"
	postmarkToken  = "postmark-test-token-" + "0123456789"
	mailgunKey     = "mailgun-test-signing-key-" + "0123456789"
	mailgunToken   = "0123456789abcdefghijklmnopqrstuvwxyz0123456789ab"
	wrongCredValue = "not-the-" + "credential"
)

func postmarkPost(t *testing.T, env *testhelper.TestEnv, key, user, password, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/hooks/"+key, strings.NewReader(body))
	if password != "" {
		req.SetBasicAuth(user, password)
	}
	rec := httptest.NewRecorder()
	env.Server.ServeHTTP(rec, req)
	return rec
}

// mailgunPost wraps eventData in Mailgun's envelope, signed with signingKey.
func mailgunPost(t *testing.T, env *testhelper.TestEnv, key, signingKey, eventData string) *httptest.ResponseRecorder {
	t.Helper()
	timestamp := strconv.FormatInt(hookClock.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(signingKey))
	mac.Write([]byte(timestamp + mailgunToken))
	body := `{"signature":{"token":"` + mailgunToken + `","timestamp":"` + timestamp + `","signature":"` + hex.EncodeToString(mac.Sum(nil)) + `"},"event-data":` + eventData + `}`
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/hooks/"+key, strings.NewReader(body))
	rec := httptest.NewRecorder()
	env.Server.ServeHTTP(rec, req)
	return rec
}

// failuresFor is every delivery failure published for an address.
func failuresFor(t *testing.T, env *testhelper.TestEnv, email string) []events.EmailDeliveryFailure {
	t.Helper()
	var out []events.EmailDeliveryFailure
	for _, ev := range env.OutboxEvents(t, events.NameEmailBounced, events.NameEmailComplained) {
		f, ok := ev.(*events.EmailDeliveryFailure)
		require.Truef(t, ok, "got %T", ev)
		if f.Email == email {
			out = append(out, *f)
		}
	}
	return out
}

func postmarkBody(recordType, id, typeName, email string) string {
	return `{"RecordType":"` + recordType + `","ID":` + id + `,"Type":"` + typeName + `","Email":"` + email + `","From":"News <hello@Codebasics.dev>"}`
}

func mailgunEvent(event, id, severity, email string) string {
	sev := ""
	if severity != "" {
		sev = `"severity":"` + severity + `",`
	}
	return `{"event":"` + event + `","id":"` + id + `",` + sev + `"recipient":"` + email + `","message":{"headers":{"from":"News <hello@Codebasics.dev>"}},"domain":{"name":"codebasics.dev"}}`
}

func TestPostmarkIntegrationSuppressesHardBouncesAndComplaintsOnly(t *testing.T) {
	env := setupHooks(t)
	key := fixtures.IntegrationAcmePostmarkIngestKey
	alice := strings.ToUpper(fixtures.ContactAliceEmail)

	require.Equal(t, http.StatusOK, postmarkPost(t, env, key, postmarkUser, postmarkToken, postmarkBody("Bounce", "101", "HardBounce", alice)).Code)
	require.Equal(t, http.StatusOK, postmarkPost(t, env, key, postmarkUser, postmarkToken, postmarkBody("Bounce", "102", "SoftBounce", "soft@example.com")).Code)
	require.Equal(t, http.StatusOK, postmarkPost(t, env, key, postmarkUser, postmarkToken, postmarkBody("SpamComplaint", "103", "SpamComplaint", "angry@example.com")).Code)
	require.Equal(t, http.StatusOK, postmarkPost(t, env, key, postmarkUser, postmarkToken, postmarkBody("Bounce", "104", "Unsubscribe", "gone@example.com")).Code)

	hard := failuresFor(t, env, fixtures.ContactAliceEmail)
	require.Len(t, hard, 1)
	assert.Equal(t, events.NameEmailBounced, hard[0].Action)
	assert.Equal(t, events.BounceKindPermanent, hard[0].BounceKind)
	assert.EqualValues(t, fixtures.ContactAliceID, hard[0].ContactID)
	assert.Equal(t, "postmark", hard[0].Provider)
	assert.Equal(t, "codebasics.dev", hard[0].SendingDomain)
	assert.Equal(t, "101/"+fixtures.ContactAliceEmail, hard[0].DedupID)

	soft := failuresFor(t, env, "soft@example.com")
	require.Len(t, soft, 1, "a soft bounce is published: the deliverability rates read it")
	assert.Equal(t, events.BounceKindTransient, soft[0].BounceKind)
	complaint := failuresFor(t, env, "angry@example.com")
	require.Len(t, complaint, 1)
	assert.Equal(t, events.NameEmailComplained, complaint[0].Action)
	assert.Empty(t, failuresFor(t, env, "gone@example.com"), "an unsubscribe request is not a delivery failure")

	applySuppression(t, env)
	assert.True(t, suppressed(t, env, fixtures.ContactAliceEmail))
	assert.True(t, suppressed(t, env, "angry@example.com"))
	assert.False(t, suppressed(t, env, "soft@example.com"))
	assert.False(t, suppressed(t, env, "gone@example.com"))
}

func TestPostmarkIntegrationRefusesWrongCredentialsAndChangesNothing(t *testing.T) {
	env := setupHooks(t)
	key := fixtures.IntegrationAcmePostmarkIngestKey
	body := postmarkBody("Bounce", "201", "HardBounce", fixtures.ContactAliceEmail)

	assert.Equal(t, http.StatusForbidden, postmarkPost(t, env, key, postmarkUser, wrongCredValue, body).Code)
	assert.Equal(t, http.StatusForbidden, postmarkPost(t, env, key, wrongCredValue, postmarkToken, body).Code)
	assert.Equal(t, http.StatusForbidden, postmarkPost(t, env, key, "", "", body).Code)

	assert.Empty(t, failuresFor(t, env, fixtures.ContactAliceEmail))
	applySuppression(t, env)
	assert.False(t, suppressed(t, env, fixtures.ContactAliceEmail))
}

func TestMailgunIntegrationSuppressesPermanentFailuresAndComplaintsOnly(t *testing.T) {
	env := setupHooks(t)
	key := fixtures.IntegrationAcmeMailgunIngestKey
	alice := strings.ToUpper(fixtures.ContactAliceEmail)

	require.Equal(t, http.StatusOK, mailgunPost(t, env, key, mailgunKey, mailgunEvent("failed", "evt-hard", "permanent", alice)).Code)
	require.Equal(t, http.StatusOK, mailgunPost(t, env, key, mailgunKey, mailgunEvent("failed", "evt-soft", "temporary", "soft@example.com")).Code)
	require.Equal(t, http.StatusOK, mailgunPost(t, env, key, mailgunKey, mailgunEvent("complained", "evt-spam", "", "angry@example.com")).Code)
	require.Equal(t, http.StatusOK, mailgunPost(t, env, key, mailgunKey, mailgunEvent("delivered", "evt-ok", "", "fine@example.com")).Code)

	hard := failuresFor(t, env, fixtures.ContactAliceEmail)
	require.Len(t, hard, 1)
	assert.Equal(t, events.BounceKindPermanent, hard[0].BounceKind)
	assert.EqualValues(t, fixtures.ContactAliceID, hard[0].ContactID)
	assert.Equal(t, "mailgun", hard[0].Provider)
	assert.Equal(t, "codebasics.dev", hard[0].SendingDomain)
	assert.Equal(t, "evt-hard/"+fixtures.ContactAliceEmail, hard[0].DedupID)

	soft := failuresFor(t, env, "soft@example.com")
	require.Len(t, soft, 1, "a temporary failure is published: the deliverability rates read it")
	assert.Equal(t, events.BounceKindTransient, soft[0].BounceKind)
	complaint := failuresFor(t, env, "angry@example.com")
	require.Len(t, complaint, 1)
	assert.Equal(t, events.NameEmailComplained, complaint[0].Action)
	assert.Empty(t, failuresFor(t, env, "fine@example.com"), "a delivered event is not a failure")

	applySuppression(t, env)
	assert.True(t, suppressed(t, env, fixtures.ContactAliceEmail))
	assert.True(t, suppressed(t, env, "angry@example.com"))
	assert.False(t, suppressed(t, env, "soft@example.com"))
}

func TestMailgunIntegrationRefusesAWrongSigningKeyAndChangesNothing(t *testing.T) {
	env := setupHooks(t)
	key := fixtures.IntegrationAcmeMailgunIngestKey

	rec := mailgunPost(t, env, key, wrongCredValue, mailgunEvent("failed", "evt-1", "permanent", fixtures.ContactAliceEmail))

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Empty(t, failuresFor(t, env, fixtures.ContactAliceEmail))
	applySuppression(t, env)
	assert.False(t, suppressed(t, env, fixtures.ContactAliceEmail))
}

// A request authentic for one Integration is refused at another's URL, whatever the Preset.
func TestACallbackAuthenticForOneIntegrationIsRefusedAtAnother(t *testing.T) {
	env := setupHooks(t)
	body := postmarkBody("Bounce", "301", "HardBounce", fixtures.ContactAliceEmail)
	event := mailgunEvent("failed", "evt-301", "permanent", fixtures.ContactAliceEmail)

	assert.Equal(t, http.StatusForbidden, postmarkPost(t, env, fixtures.IntegrationAcmeMailgunIngestKey, postmarkUser, postmarkToken, body).Code)
	assert.Equal(t, http.StatusForbidden, mailgunPost(t, env, fixtures.IntegrationAcmePostmarkIngestKey, mailgunKey, event).Code)
	assert.Equal(t, http.StatusForbidden, mailgunPost(t, env, fixtures.IntegrationAcmeGenericAIngestKey, mailgunKey, event).Code)

	assert.Empty(t, failuresFor(t, env, fixtures.ContactAliceEmail))
}
