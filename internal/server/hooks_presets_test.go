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

	"github.com/getsphericon/sphericon/ent"
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
func failuresFor(t *testing.T, env *testhelper.TestEnv, email string) []events.EmailDelivery {
	t.Helper()
	var out []events.EmailDelivery
	for _, ev := range env.OutboxEvents(t, deliveryNames...) {
		f, ok := ev.(*events.EmailDelivery)
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

// postmarkBodyFor is a Postmark Bounce or SpamComplaint payload that names the message by
// Postmark's MessageID.
func postmarkBodyFor(recordType, id, typeName, email, messageID string) string {
	return `{"RecordType":"` + recordType + `","ID":` + id + `,"Type":"` + typeName + `","MessageID":"` + messageID + `","Email":"` + email + `","From":"News <hello@Codebasics.dev>"}`
}

// postmarkDelivery is a Postmark Delivery payload (it has Recipient and no From).
func postmarkDelivery(email, messageID string) string {
	return `{"RecordType":"Delivery","MessageStream":"outbound","MessageID":"` + messageID + `","Recipient":"` + email + `"}`
}

func mailgunEvent(event, id, severity, email string) string {
	return mailgunEventFor(event, id, severity, email, "")
}

// mailgunEventFor names the message by its Message-ID header, which Mailgun reports without
// angle brackets (messageID "" leaves the header out).
func mailgunEventFor(event, id, severity, email, messageID string) string {
	sev := ""
	if severity != "" {
		sev = `"severity":"` + severity + `",`
	}
	mid := ""
	if messageID != "" {
		mid = `,"message-id":"` + messageID + `"`
	}
	return `{"event":"` + event + `","id":"` + id + `",` + sev + `"recipient":"` + email + `","message":{"headers":{"from":"News <hello@Codebasics.dev>"` + mid + `}},"domain":{"name":"codebasics.dev"}}`
}

func messageOf(t *testing.T, env *testhelper.TestEnv, id int64) *ent.OutboundMessage {
	t.Helper()
	m, err := env.DB.OutboundMessage.Get(t.Context(), id)
	require.NoError(t, err)
	return m
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
	assert.Equal(t, events.BounceKindHard, hard[0].BounceKind)
	assert.EqualValues(t, fixtures.ContactAliceID, hard[0].ContactID)
	assert.Equal(t, "postmark", hard[0].Provider)
	assert.Equal(t, "codebasics.dev", hard[0].SendingDomain)
	assert.Equal(t, "101/"+fixtures.ContactAliceEmail, hard[0].DedupID)

	soft := failuresFor(t, env, "soft@example.com")
	require.Len(t, soft, 1, "a soft bounce is published: the deliverability rates read it")
	assert.Equal(t, events.BounceKindSoft, soft[0].BounceKind)
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

// A verified request our Preset cannot read is our failure, not the provider's: 5xx, so the
// provider retries once the Preset is fixed, and nothing is published.
func TestAVerifiedRequestTheTransformCannotReadAnswers5xxAndChangesNothing(t *testing.T) {
	env := setupHooks(t)
	body := `{"RecordType":"Bounce","ID":202,"Type":"HardBounce","From":"News <hello@Codebasics.dev>"}`

	rec := postmarkPost(t, env, fixtures.IntegrationAcmePostmarkIngestKey, postmarkUser, postmarkToken, body)

	assert.GreaterOrEqual(t, rec.Code, http.StatusInternalServerError)
	assert.Empty(t, env.OutboxEvents(t, deliveryNames...))
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
	assert.Equal(t, events.BounceKindHard, hard[0].BounceKind)
	assert.EqualValues(t, fixtures.ContactAliceID, hard[0].ContactID)
	assert.Equal(t, "mailgun", hard[0].Provider)
	assert.Equal(t, "codebasics.dev", hard[0].SendingDomain)
	assert.Equal(t, "evt-hard/"+fixtures.ContactAliceEmail, hard[0].DedupID)

	soft := failuresFor(t, env, "soft@example.com")
	require.Len(t, soft, 1, "a temporary failure is Mailgun retrying: a delay")
	assert.Equal(t, events.NameEmailDelayed, soft[0].Action)
	complaint := failuresFor(t, env, "angry@example.com")
	require.Len(t, complaint, 1)
	assert.Equal(t, events.NameEmailComplained, complaint[0].Action)
	fine := failuresFor(t, env, "fine@example.com")
	require.Len(t, fine, 1, "a delivered event is a delivery notice")
	assert.Equal(t, events.NameEmailDelivered, fine[0].Action)

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

// Postmark and Mailgun produce the same Events and stamps as SES (hooks_delivery_test.go): a
// notice that names a sent message links to it and stamps its delivery result once.

func TestPostmarkDeliveryNoticeLinksAndStampsTheMessage(t *testing.T) {
	env := setupHooks(t)
	key := fixtures.IntegrationAcmePostmarkIngestKey

	require.Equal(t, http.StatusOK, postmarkPost(t, env, key, postmarkUser, postmarkToken, postmarkDelivery(fixtures.ContactAliceEmail, "pm-msg-alice-1")).Code)

	got := failuresFor(t, env, fixtures.ContactAliceEmail)
	require.Len(t, got, 1)
	assert.Equal(t, events.NameEmailDelivered, got[0].Action)
	assert.Equal(t, "postmark", got[0].Provider)
	assert.Equal(t, "pm-msg-alice-1", got[0].ProviderMessageID)
	assert.EqualValues(t, fixtures.OutboundMessagePostmarkAliceID, got[0].OutboundMessageID)
	assert.EqualValues(t, fixtures.ContactAliceID, got[0].ContactID)
	m := messageOf(t, env, fixtures.OutboundMessagePostmarkAliceID)
	require.NotNil(t, m.DeliveredAt)
	assert.True(t, hookClock.Equal(*m.DeliveredAt))
	assert.Nil(t, m.BouncedAt)
	assert.Nil(t, m.ComplainedAt)
}

func TestPostmarkTransientIsADelayAndBounceAndComplaintStamp(t *testing.T) {
	env := setupHooks(t)
	key := fixtures.IntegrationAcmePostmarkIngestKey

	require.Equal(t, http.StatusOK, postmarkPost(t, env, key, postmarkUser, postmarkToken, postmarkBodyFor("Bounce", "401", "Transient", fixtures.ContactAliceEmail, "pm-msg-alice-1")).Code)
	got := failuresFor(t, env, fixtures.ContactAliceEmail)
	require.Len(t, got, 1)
	assert.Equal(t, events.NameEmailDelayed, got[0].Action)
	assert.EqualValues(t, fixtures.OutboundMessagePostmarkAliceID, got[0].OutboundMessageID)
	m := messageOf(t, env, fixtures.OutboundMessagePostmarkAliceID)
	assert.Nil(t, m.DeliveredAt)
	assert.Nil(t, m.BouncedAt)

	require.Equal(t, http.StatusOK, postmarkPost(t, env, key, postmarkUser, postmarkToken, postmarkBodyFor("Bounce", "402", "HardBounce", fixtures.ContactAliceEmail, "pm-msg-alice-1")).Code)
	require.Equal(t, http.StatusOK, postmarkPost(t, env, key, postmarkUser, postmarkToken, postmarkBodyFor("SpamComplaint", "403", "SpamComplaint", fixtures.ContactAliceEmail, "pm-msg-alice-1")).Code)
	m = messageOf(t, env, fixtures.OutboundMessagePostmarkAliceID)
	assert.NotNil(t, m.BouncedAt)
	assert.NotNil(t, m.ComplainedAt)
	applySuppression(t, env)
	assert.True(t, suppressed(t, env, fixtures.ContactAliceEmail))
}

func TestPostmarkDeliveryWithoutAMatchingMessageStaysAtTheAddress(t *testing.T) {
	env := setupHooks(t)

	require.Equal(t, http.StatusOK, postmarkPost(t, env, fixtures.IntegrationAcmePostmarkIngestKey, postmarkUser, postmarkToken, postmarkDelivery(fixtures.ContactAliceEmail, "pm-msg-unknown")).Code)

	got := failuresFor(t, env, fixtures.ContactAliceEmail)
	require.Len(t, got, 1)
	assert.Zero(t, got[0].OutboundMessageID)
	assert.Nil(t, messageOf(t, env, fixtures.OutboundMessagePostmarkAliceID).DeliveredAt)
}

func TestMailgunNoticesLinkToTheMessageByItsMessageIDAndStampIt(t *testing.T) {
	env := setupHooks(t)
	key := fixtures.IntegrationAcmeMailgunIngestKey
	const mid = "mg-msg-alice-1@codebasics.dev"

	require.Equal(t, http.StatusOK, mailgunPost(t, env, key, mailgunKey, mailgunEventFor("delivered", "evt-d", "", fixtures.ContactAliceEmail, mid)).Code)
	got := failuresFor(t, env, fixtures.ContactAliceEmail)
	require.Len(t, got, 1)
	assert.Equal(t, events.NameEmailDelivered, got[0].Action)
	assert.Equal(t, "mailgun", got[0].Provider)
	assert.Equal(t, "<"+mid+">", got[0].ProviderMessageID)
	assert.EqualValues(t, fixtures.OutboundMessageMailgunAliceID, got[0].OutboundMessageID)
	m := messageOf(t, env, fixtures.OutboundMessageMailgunAliceID)
	require.NotNil(t, m.DeliveredAt)
	assert.True(t, hookClock.Equal(*m.DeliveredAt))

	require.Equal(t, http.StatusOK, mailgunPost(t, env, key, mailgunKey, mailgunEventFor("failed", "evt-b", "permanent", fixtures.ContactAliceEmail, mid)).Code)
	require.Equal(t, http.StatusOK, mailgunPost(t, env, key, mailgunKey, mailgunEventFor("complained", "evt-c", "", fixtures.ContactAliceEmail, mid)).Code)
	m = messageOf(t, env, fixtures.OutboundMessageMailgunAliceID)
	assert.NotNil(t, m.BouncedAt)
	assert.NotNil(t, m.ComplainedAt)
}

func TestMailgunDelayAndDropAreEventsAndStampNothing(t *testing.T) {
	env := setupHooks(t)
	key := fixtures.IntegrationAcmeMailgunIngestKey
	const mid = "mg-msg-alice-1@codebasics.dev"

	require.Equal(t, http.StatusOK, mailgunPost(t, env, key, mailgunKey, mailgunEventFor("failed", "evt-t", "temporary", fixtures.ContactAliceEmail, mid)).Code)
	require.Equal(t, http.StatusOK, mailgunPost(t, env, key, mailgunKey, mailgunEventFor("rejected", "evt-r", "", fixtures.ContactAliceEmail, mid)).Code)

	byAction := map[string]events.EmailDelivery{}
	for _, ev := range failuresFor(t, env, fixtures.ContactAliceEmail) {
		byAction[ev.Action] = ev
	}
	require.Len(t, byAction, 2)
	assert.Contains(t, byAction, events.NameEmailDelayed)
	assert.Contains(t, byAction, events.NameEmailDropped)
	for _, ev := range byAction {
		assert.EqualValues(t, fixtures.OutboundMessageMailgunAliceID, ev.OutboundMessageID)
	}
	m := messageOf(t, env, fixtures.OutboundMessageMailgunAliceID)
	assert.Nil(t, m.DeliveredAt)
	assert.Nil(t, m.BouncedAt)
	assert.Nil(t, m.ComplainedAt)
	applySuppression(t, env)
	assert.False(t, suppressed(t, env, fixtures.ContactAliceEmail))
}
