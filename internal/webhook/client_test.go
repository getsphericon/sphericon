package webhook_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	standardwebhooks "github.com/standard-webhooks/standard-webhooks/libraries/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/webhook"
)

// A valid Standard Webhooks secret (whsec_ + base64), from the spec's test vectors.
const testSecret = "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"

// receiver is an httptest server on loopback together with the allowlist that permits it.
func receiver(t *testing.T, h http.HandlerFunc) (*httptest.Server, webhook.Allowlist) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	allow, err := webhook.ParseAllowlist(u.Host)
	require.NoError(t, err)
	return srv, allow
}

func ok(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

// Without an allowlist the hardened client refuses loopback (and by extension private and
// link-local) addresses, checked on the resolved IP: a name that resolves to loopback is
// refused too.
func TestClientRefusesLoopbackWithoutAllowlist(t *testing.T) {
	srv, _ := receiver(t, ok)
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	byName := "http://localhost:" + u.Port()

	for _, target := range []string{srv.URL, byName} {
		err := webhook.Send(t.Context(), webhook.NewClient(2*time.Second), target, testSecret, "x", "msg_1", []byte(`{}`))
		require.Error(t, err, target)
	}
}

// A host:port the allowlist names is reached; the receiver verifies the Standard Webhooks
// signature with the same secret.
func TestClientReachesAnAllowlistedReceiverAndItVerifiesTheSignature(t *testing.T) {
	body := []byte(`{"id":"evt_1","type":"contact.created"}`)
	var verifyErr error
	var gotEvent string
	srv, allow := receiver(t, func(w http.ResponseWriter, r *http.Request) {
		gotEvent = r.Header.Get(webhook.EventHeader)
		received, _ := io.ReadAll(r.Body)
		wh, _ := standardwebhooks.NewWebhook(testSecret)
		verifyErr = wh.Verify(received, r.Header)
		w.WriteHeader(http.StatusAccepted)
	})

	err := webhook.Send(t.Context(), webhook.NewClientAllowing(2*time.Second, allow), srv.URL, testSecret, "contact.created", "evt_1", body)
	require.NoError(t, err)
	require.NoError(t, verifyErr)
	assert.Equal(t, "contact.created", gotEvent)
}

// The allowlist names a port too: the same host on another port stays refused.
func TestAllowlistNamesThePortToo(t *testing.T) {
	srv, _ := receiver(t, ok)
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	other := "127.0.0.1:1"
	if u.Port() == "1" {
		other = "127.0.0.1:2"
	}
	allow, err := webhook.ParseAllowlist(other)
	require.NoError(t, err)

	err = webhook.Send(t.Context(), webhook.NewClientAllowing(2*time.Second, allow), srv.URL, testSecret, "x", "msg_1", []byte(`{}`))
	require.Error(t, err)
}

// A redirect is surfaced, never followed, even from an allowlisted host.
func TestClientDoesNotFollowRedirects(t *testing.T) {
	var hits int
	target, _ := receiver(t, func(w http.ResponseWriter, _ *http.Request) { hits++; w.WriteHeader(http.StatusOK) })
	srv, allow := receiver(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	})

	err := webhook.Send(t.Context(), webhook.NewClientAllowing(2*time.Second, allow), srv.URL, testSecret, "x", "msg_1", []byte(`{}`))
	require.ErrorContains(t, err, "302")
	assert.Zero(t, hits)
}

func TestSendErrorsOnNon2xx(t *testing.T) {
	srv, allow := receiver(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) })

	err := webhook.Send(t.Context(), webhook.NewClientAllowing(2*time.Second, allow), srv.URL, testSecret, "x", "msg_1", []byte(`{}`))
	require.ErrorContains(t, err, "500")
}

func TestParseAllowlistRejectsMalformedValues(t *testing.T) {
	for _, bad := range []string{"n8n", "n8n:", "n8n:0", "n8n:70000", "n8n:http", ":5678", "*.lan:5678", "10.0.0.0/8:80", "n8n:5678,oops"} {
		_, err := webhook.ParseAllowlist(bad)
		assert.Error(t, err, bad)
	}
}

func TestParseAllowlistAcceptsNamesAndAddresses(t *testing.T) {
	_, err := webhook.ParseAllowlist("n8n:5678, 10.0.0.5:8080 [::1]:9000\nnode-red.lan:1880")
	require.NoError(t, err)
	_, err = webhook.ParseAllowlist("")
	require.NoError(t, err)
}
