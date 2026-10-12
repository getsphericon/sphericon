package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/server"
	"github.com/getsphericon/sphericon/internal/testhelper"
	sns "github.com/robbiet480/go.sns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An unknown ingest key is rejected (404) before any signature work — the
// workspace is resolved from the key first.
func TestSESHookUnknownKeyNotFound(t *testing.T) {
	env := testhelper.Setup(t)

	req := httptest.NewRequestWithContext(t.Context(), "POST", "/hooks/omik_does_not_exist/ses",
		strings.NewReader(`{"Type":"Notification","Message":"{}"}`))
	rec := httptest.NewRecorder()
	env.Server.ServeHTTP(rec, req)

	assert.Equal(t, 404, rec.Code)
}

// A known workspace key with an unsigned payload is rejected by SNS signature
// verification (403), confirming the verify gate is wired ahead of processing.
func TestSESHookRejectsUnverifiedPayload(t *testing.T) {
	env := testhelper.Setup(t)

	// The Acme workspace ingest key resolves the workspace before verification.
	req := httptest.NewRequestWithContext(t.Context(), "POST", "/hooks/"+fixtures.AcmeIngestKey+"/ses",
		strings.NewReader(`{"Type":"Notification","Message":"{}","Signature":"bogus","SigningCertURL":"https://sns.us-east-1.amazonaws.com/x.pem"}`))
	rec := httptest.NewRecorder()
	env.Server.ServeHTTP(rec, req)

	assert.Equal(t, 403, rec.Code)
}

func postSES(t *testing.T, h http.Handler, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), "POST", "/hooks/"+key+"/ses", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func sesNotificationBody(t *testing.T, message string) string {
	t.Helper()
	b, err := json.Marshal(map[string]string{"Type": "Notification", "MessageId": "sns-msg-1", "Message": message})
	require.NoError(t, err)
	return string(b)
}

func acceptAll(*sns.Payload) error { return nil }

func noConfirm(context.Context, string) error { return nil }

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

// A permanent bounce for a known contact publishes one failure event linked to it,
// keyed by SNS message id + recipient, with the From domain echoed.
func TestSESHookPublishesPermanentBounce(t *testing.T) {
	env := testhelper.Setup(t)
	h := server.NewSESHooks(env.DB, env.Bus, acceptAll, noConfirm)

	msg := `{"notificationType":"Bounce","mail":{"source":"News <hello@Codebasics.dev>"},
	  "bounce":{"bounceType":"Permanent","bouncedRecipients":[{"emailAddress":"` + strings.ToUpper(fixtures.ContactAliceEmail) + `"}]}}`
	rec := postSES(t, h, fixtures.AcmeIngestKey, sesNotificationBody(t, msg))
	require.Equal(t, 200, rec.Code)

	got := outboxEvents(t, env, fixtures.ContactAliceEmail)
	require.Len(t, got, 1)
	assert.Equal(t, events.NameEmailBounced, got[0].Action)
	assert.Equal(t, events.BounceKindPermanent, got[0].BounceKind)
	assert.EqualValues(t, fixtures.ContactAliceID, got[0].ContactID)
	assert.Equal(t, "codebasics.dev", got[0].SendingDomain)
	assert.Equal(t, "ses", got[0].Provider)
	assert.Equal(t, "sns-msg-1/"+fixtures.ContactAliceEmail, got[0].DedupID)
}

// A complaint for an address that is not a contact is still published (so it is
// suppressed), just without a contact link; a blank recipient is skipped.
func TestSESHookPublishesComplaintForUnknownAddress(t *testing.T) {
	env := testhelper.Setup(t)
	h := server.NewSESHooks(env.DB, env.Bus, acceptAll, noConfirm)

	msg := `{"eventType":"Complaint","complaint":{"complainedRecipients":[{"emailAddress":" Stranger@Example.com "},{"emailAddress":"  "}]}}`
	require.Equal(t, 200, postSES(t, h, fixtures.AcmeIngestKey, sesNotificationBody(t, msg)).Code)

	got := outboxEvents(t, env, "stranger@example.com")
	require.Len(t, got, 1)
	assert.Equal(t, events.NameEmailComplained, got[0].Action)
	assert.Zero(t, got[0].ContactID)
}

func TestSESHookNotificationErrors(t *testing.T) {
	env := testhelper.Setup(t)
	h := server.NewSESHooks(env.DB, env.Bus, acceptAll, noConfirm)

	// A Message that is not JSON is a 500 so SNS redelivers it.
	assert.Equal(t, 500, postSES(t, h, fixtures.AcmeIngestKey, sesNotificationBody(t, "not json")).Code)
	// A body that is not an SNS payload is a 400.
	assert.Equal(t, 400, postSES(t, h, fixtures.AcmeIngestKey, "not json").Code)
	// Delivery notifications carry no failure: accepted, nothing published.
	assert.Equal(t, 200, postSES(t, h, fixtures.AcmeIngestKey, sesNotificationBody(t, `{"notificationType":"Delivery"}`)).Code)
}

func TestSESHookRejectsFailedVerification(t *testing.T) {
	env := testhelper.Setup(t)
	h := server.NewSESHooks(env.DB, env.Bus, func(*sns.Payload) error { return errors.New("bad sig") }, noConfirm)
	assert.Equal(t, 403, postSES(t, h, fixtures.AcmeIngestKey, sesNotificationBody(t, "{}")).Code)
}

func TestSESHookSubscriptionConfirmation(t *testing.T) {
	env := testhelper.Setup(t)
	body := `{"Type":"SubscriptionConfirmation","SubscribeURL":"https://sns.us-east-1.amazonaws.com/?Action=ConfirmSubscription"}`

	var confirmed string
	ok := server.NewSESHooks(env.DB, env.Bus, acceptAll, func(_ context.Context, u string) error { confirmed = u; return nil })
	assert.Equal(t, 200, postSES(t, ok, fixtures.AcmeIngestKey, body).Code)
	assert.Contains(t, confirmed, "ConfirmSubscription")

	// A failed confirmation is a 502 so SNS resends it.
	failing := server.NewSESHooks(env.DB, env.Bus, acceptAll, func(context.Context, string) error { return errors.New("down") })
	assert.Equal(t, 502, postSES(t, failing, fixtures.AcmeIngestKey, body).Code)
}

// The real confirmer only follows https amazonaws.com URLs.
func TestConfirmSNSSubscriptionRefusesNonAWSURLs(t *testing.T) {
	for _, u := range []string{
		"http://sns.us-east-1.amazonaws.com/x",
		"https://evil.example.com/x",
		"https://amazonaws.com.evil.example/x",
		"://bad",
	} {
		require.Error(t, server.ConfirmSNSSubscription(t.Context(), u), u)
	}
}
