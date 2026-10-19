package server_test

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/testhelper/metricstest"
)

// The hook over HTTP for an Integration on the `ses` Preset (ADR 0037): Amazon SNS posts
// the SES notification as text/plain. The notifications in testdata/sns are signed with
// the test certificate the test environment's registry trusts (preset.TestSNSCertificates),
// for the topic the Acme SES Integration names in its params. The Preset's own `tests` hold
// the transform and verification cases; these tests hold what the hook leaves behind.

// snsNotification reads a signed SNS body from testdata/sns.
func snsNotification(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "sns", name+".json"))
	require.NoError(t, err)
	return string(b)
}

func postSNS(t *testing.T, env *testhelper.TestEnv, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/hooks/"+key, strings.NewReader(body))
	req.Header.Set("Content-Type", "text/plain; charset=UTF-8")
	rec := httptest.NewRecorder()
	env.Server.ServeHTTP(rec, req)
	return rec
}

// outboxEvents returns the published delivery-failure events for an address.
func outboxEvents(t *testing.T, env *testhelper.TestEnv, email string) []events.EmailDeliveryFailure {
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

// persistOutbox plays the persist consumer over everything published so far.
func persistOutbox(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	for _, envelope := range env.OutboxEnvelopes(t, events.NameEmailBounced, events.NameEmailComplained) {
		require.NoError(t, events.Persist(t.Context(), env.DB, envelope))
	}
}

// A permanent bounce for a known Contact publishes one failure linked to it, keyed by the
// SNS message id plus the recipient, with the From domain echoed, and suppresses it.
func TestSESHookSuppressesAPermanentBounce(t *testing.T) {
	env := setupHooks(t)

	rec := postSNS(t, env, fixtures.IntegrationAcmeSesIngestKey, snsNotification(t, "permanent-bounce"))
	require.Equal(t, http.StatusOK, rec.Code)

	got := outboxEvents(t, env, fixtures.ContactAliceEmail)
	require.Len(t, got, 1)
	assert.Equal(t, events.NameEmailBounced, got[0].Action)
	assert.Equal(t, events.BounceKindPermanent, got[0].BounceKind)
	assert.EqualValues(t, fixtures.ContactAliceID, got[0].ContactID)
	assert.Equal(t, "codebasics.dev", got[0].SendingDomain)
	assert.Equal(t, "ses", got[0].Provider)
	assert.Equal(t, "sns-msg-1/"+fixtures.ContactAliceEmail, got[0].DedupID)

	applySuppression(t, env)
	assert.True(t, suppressed(t, env, fixtures.ContactAliceEmail))
}

// A transient bounce is published (the rates read it) and never suppresses.
func TestSESHookRecordsATransientBounceWithoutSuppressing(t *testing.T) {
	env := setupHooks(t)

	require.Equal(t, http.StatusOK, postSNS(t, env, fixtures.IntegrationAcmeSesIngestKey, snsNotification(t, "transient-bounce")).Code)

	got := outboxEvents(t, env, "soft@example.com")
	require.Len(t, got, 1)
	assert.Equal(t, events.BounceKindTransient, got[0].BounceKind)
	applySuppression(t, env)
	assert.False(t, suppressed(t, env, "soft@example.com"))
}

// A complaint for an address that is no Contact is still published (so it is suppressed),
// without a Contact link.
func TestSESHookSuppressesAComplaintForAnUnknownAddress(t *testing.T) {
	env := setupHooks(t)

	require.Equal(t, http.StatusOK, postSNS(t, env, fixtures.IntegrationAcmeSesIngestKey, snsNotification(t, "complaint")).Code)

	got := outboxEvents(t, env, "stranger@example.com")
	require.Len(t, got, 1)
	assert.Equal(t, events.NameEmailComplained, got[0].Action)
	assert.Zero(t, got[0].ContactID)
	applySuppression(t, env)
	assert.True(t, suppressed(t, env, "stranger@example.com"))
}

// A signed notification from a topic the Integration did not name is refused and changes
// nothing: an SNS signature proves AWS sent it, not whose topic it came from.
func TestSESHookRefusesANotificationFromAnotherTopic(t *testing.T) {
	env := setupHooks(t)

	rec := postSNS(t, env, fixtures.IntegrationAcmeSesIngestKey, snsNotification(t, "other-topic"))

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Empty(t, outboxEvents(t, env, "victim@example.com"))
	applySuppression(t, env)
	assert.False(t, suppressed(t, env, "victim@example.com"))
}

// An SES Integration that names no topic fails closed, even for a notification that is
// validly signed.
func TestSESHookFailsClosedWithoutATopic(t *testing.T) {
	env := setupHooks(t)

	rec := postSNS(t, env, fixtures.IntegrationGlobexSesNoTopicIngestKey, snsNotification(t, "permanent-bounce"))

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Empty(t, outboxEvents(t, env, fixtures.ContactAliceEmail))
}

// A notification that is not signed by AWS, or is not SNS at all, changes nothing.
func TestSESHookRefusesAnUnsignedOrUnreadableRequest(t *testing.T) {
	env := setupHooks(t)
	key := fixtures.IntegrationAcmeSesIngestKey

	var notification map[string]string
	require.NoError(t, json.Unmarshal([]byte(snsNotification(t, "permanent-bounce")), &notification))
	notification["Signature"] = "Ym9ndXM="
	forged, err := json.Marshal(notification)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, postSNS(t, env, key, string(forged)).Code)
	assert.Equal(t, http.StatusBadRequest, postSNS(t, env, key, "not json").Code)

	assert.Empty(t, outboxEvents(t, env, fixtures.ContactAliceEmail))
}

// A subscription confirmation is performed; one that fails is a 5xx so SNS resends it.
func TestSESHookConfirmsASubscription(t *testing.T) {
	env := setupHooks(t)
	key := fixtures.IntegrationAcmeSesIngestKey
	body := snsNotification(t, "confirmation")

	require.Equal(t, http.StatusOK, postSNS(t, env, key, body).Code)
	require.Len(t, env.Confirmations.URLs(), 1)
	assert.Contains(t, env.Confirmations.URLs()[0], "Action=ConfirmSubscription")

	env.Confirmations.Fail(errors.New("down"))
	assert.Equal(t, http.StatusBadGateway, postSNS(t, env, key, body).Code)
}

// SES bounces and complaints are counted by provider and status, and an SNS redelivery
// of the same notification is persisted and counted once.
func TestSESHookCountsOutcomesAndDedupesRedelivery(t *testing.T) {
	env := setupHooks(t)
	metricstest.StartMetrics(t)
	key := fixtures.IntegrationAcmeSesIngestKey
	bounce := snsNotification(t, "permanent-bounce")

	require.Equal(t, http.StatusOK, postSNS(t, env, key, bounce).Code)
	require.Equal(t, http.StatusOK, postSNS(t, env, key, bounce).Code)
	require.Equal(t, http.StatusOK, postSNS(t, env, key, snsNotification(t, "complaint")).Code)
	persistOutbox(t, env)

	n, err := env.DB.Event.Query().Where(event.SourceID("sns-msg-1/" + fixtures.ContactAliceEmail)).Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	scrape := metricstest.ScrapeMetrics(t)
	assert.InDelta(t, 1, scrape.Value(t, "email_send_outcomes_total", map[string]string{"provider": "ses", "status": "complaint"}), 0)
}

// An SES Integration that names no Preset (made before Presets existed) reads its callbacks
// with `ses`: the Preset is implied by the provider.
func TestSESHookIsImpliedForAnSESIntegrationWithNoPreset(t *testing.T) {
	env := setupHooks(t)

	rec := postSNS(t, env, fixtures.IntegrationAcmeSesImpliedIngestKey, snsNotification(t, "permanent-bounce"))
	require.Equal(t, http.StatusOK, rec.Code)

	got := outboxEvents(t, env, fixtures.ContactAliceEmail)
	require.Len(t, got, 1)
	assert.Equal(t, "ses", got[0].Provider)
}

// A certificate we cannot fetch is our failure, not an unauthentic request: 5xx, so SNS
// retries, and nothing is published.
func TestSESHookAnswers5xxWhenTheCertificateIsUnavailable(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithSNSCertificates(func(context.Context, string) (*x509.Certificate, error) {
		return nil, errors.New("certificate host unreachable")
	}))

	rec := postSNS(t, env, fixtures.IntegrationAcmeSesIngestKey, snsNotification(t, "permanent-bounce"))

	assert.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Empty(t, outboxEvents(t, env, fixtures.ContactAliceEmail))
}

// A signing certificate on a host that is not Amazon's is the sender's fault: 4xx, and the
// certificate is never fetched.
func TestSESHookRefusesACertificateOnAnotherHost(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithSNSCertificates(func(context.Context, string) (*x509.Certificate, error) {
		return nil, errors.New("must not be fetched")
	}))
	var notification map[string]string
	require.NoError(t, json.Unmarshal([]byte(snsNotification(t, "permanent-bounce")), &notification))
	notification["SigningCertURL"] = "https://evil.example.com/SimpleNotificationService.pem"
	forged, err := json.Marshal(notification)
	require.NoError(t, err)

	rec := postSNS(t, env, fixtures.IntegrationAcmeSesIngestKey, string(forged))

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Empty(t, outboxEvents(t, env, fixtures.ContactAliceEmail))
}

// A callback authentic for one Preset is refused at an Integration of the other, both ways.
func TestSESAndGenericIntegrationsRefuseEachOthersCallbacks(t *testing.T) {
	env := setupHooks(t)
	generic := fixtures.IntegrationAcmeGenericAIngestKey
	ses := fixtures.IntegrationAcmeSesIngestKey

	assert.Equal(t, http.StatusForbidden, postSNS(t, env, generic, snsNotification(t, "permanent-bounce")).Code)
	assert.Equal(t, http.StatusForbidden, signedPost(t, env, ses, secretA, "msg_1", failureBody("evt_1", fixtures.ContactAliceEmail, true)).Code)
	assert.Empty(t, outboxEvents(t, env, fixtures.ContactAliceEmail))

	require.Equal(t, http.StatusOK, postSNS(t, env, ses, snsNotification(t, "permanent-bounce")).Code)
	require.Equal(t, http.StatusOK, signedPost(t, env, generic, secretA, "msg_2", failureBody("evt_2", fixtures.ContactAliceEmail, true)).Code)
	assert.Len(t, outboxEvents(t, env, fixtures.ContactAliceEmail), 2)
}
