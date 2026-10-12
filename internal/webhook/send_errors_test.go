package webhook_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/webhook"
)

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }

func TestSendRejectsAnInvalidSecret(t *testing.T) {
	called := false
	client := doerFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, errors.New("must not be reached")
	})

	err := webhook.Send(context.Background(), client, "http://example.test/hook", "whsec_%%%not-base64", "contact.created", "evt_1", []byte(`{}`))
	require.ErrorContains(t, err, "webhook secret")
	assert.False(t, called, "an unsignable delivery is never attempted")
}

func TestSendRejectsAnInvalidURL(t *testing.T) {
	client := doerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("must not be reached")
	})
	err := webhook.Send(context.Background(), client, "http://bad host/\x7f", testSecret, "contact.created", "evt_1", []byte(`{}`))
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "must not be reached")
}

func TestSendReturnsTransportErrors(t *testing.T) {
	boom := errors.New("connection reset")
	client := doerFunc(func(*http.Request) (*http.Response, error) { return nil, boom })

	err := webhook.Send(context.Background(), client, "http://example.test/hook", testSecret, "contact.created", "evt_1", []byte(`{}`))
	assert.ErrorIs(t, err, boom)
}

func TestSendSetsDeliveryHeaders(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	// Plain client: the SSRF-hardened one refuses the loopback test server.
	err := webhook.Send(context.Background(), srv.Client(), srv.URL, testSecret, "contact.created", "evt_42", []byte(`{}`))
	require.NoError(t, err)
	assert.Equal(t, "application/json", got.Get("Content-Type"))
	assert.Equal(t, "evt_42", got.Get("webhook-id"))
	assert.Equal(t, "contact.created", got.Get(webhook.EventHeader))
	assert.NotEmpty(t, got.Get("webhook-timestamp"))
	assert.NotEmpty(t, got.Get("webhook-signature"))
}
