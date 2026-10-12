package webhook_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/getsphericon/sphericon/internal/webhook"
	standardwebhooks "github.com/standard-webhooks/standard-webhooks/libraries/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A valid Standard Webhooks secret (whsec_ + base64), from the spec's test vectors.
const testSecret = "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"

// The hardened client refuses to dial loopback (and by extension private/
// link-local) addresses — the core SSRF guard. httptest binds to 127.0.0.1.
func TestNewClientBlocksLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
	require.NoError(t, err)
	resp, err := webhook.NewClient(2 * time.Second).Do(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.Error(t, err, "must refuse to dial a loopback address")
}

// Send signs with the Standard Webhooks scheme; a receiver using the same library
// verifies the delivery, and 2xx is success.
func TestSendIsVerifiableAndSucceedsOn2xx(t *testing.T) {
	body := []byte(`{"id":"evt_1","type":"contact.created"}`)

	var verifyErr error
	var gotEvent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotEvent = r.Header.Get(webhook.EventHeader)
		received, _ := io.ReadAll(r.Body)
		wh, _ := standardwebhooks.NewWebhook(testSecret)
		verifyErr = wh.Verify(received, r.Header)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	// Plain client: httptest is loopback, which the hardened client would block.
	err := webhook.Send(context.Background(), srv.Client(), srv.URL, testSecret, "contact.created", "evt_1", body)
	require.NoError(t, err)
	require.NoError(t, verifyErr, "receiver must verify the signature with the same secret")
	assert.Equal(t, "contact.created", gotEvent)
}

func TestSendErrorsOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := webhook.Send(context.Background(), srv.Client(), srv.URL, testSecret, "x", "msg_1", []byte(`{}`))
	require.Error(t, err, "non-2xx must error so river retries")
}
