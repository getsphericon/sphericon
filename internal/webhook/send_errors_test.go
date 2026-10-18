package webhook_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/webhook"
)

func TestSendRejectsAnInvalidSecretWithoutCalling(t *testing.T) {
	var hits int
	srv, allow := receiver(t, func(w http.ResponseWriter, _ *http.Request) { hits++; w.WriteHeader(http.StatusOK) })

	err := webhook.Send(t.Context(), webhook.NewClientAllowing(2*time.Second, allow), srv.URL, "whsec_%%%not-base64", "contact.created", "evt_1", []byte(`{}`))
	require.ErrorContains(t, err, "webhook secret")
	assert.Zero(t, hits, "an unsignable delivery is never attempted")
}

func TestSendRejectsAnInvalidURL(t *testing.T) {
	err := webhook.Send(t.Context(), webhook.NewClient(2*time.Second), "http://bad host/\x7f", testSecret, "contact.created", "evt_1", []byte(`{}`))
	require.Error(t, err)
}

func TestSendReturnsTransportErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	allow, err := webhook.ParseAllowlist(srv.Listener.Addr().String())
	require.NoError(t, err)
	url := srv.URL
	srv.Close()

	err = webhook.Send(t.Context(), webhook.NewClientAllowing(2*time.Second, allow), url, testSecret, "contact.created", "evt_1", []byte(`{}`))
	require.Error(t, err)
}

func TestSendSetsDeliveryHeaders(t *testing.T) {
	var got http.Header
	srv, allow := receiver(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.WriteHeader(http.StatusNoContent)
	})

	err := webhook.Send(t.Context(), webhook.NewClientAllowing(2*time.Second, allow), srv.URL, testSecret, "contact.created", "evt_42", []byte(`{}`))
	require.NoError(t, err)
	assert.Equal(t, "application/json", got.Get("Content-Type"))
	assert.Equal(t, "evt_42", got.Get("webhook-id"))
	assert.Equal(t, "contact.created", got.Get(webhook.EventHeader))
	assert.NotEmpty(t, got.Get("webhook-timestamp"))
	assert.NotEmpty(t, got.Get("webhook-signature"))
}
