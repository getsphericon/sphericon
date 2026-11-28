package webhook_test

import (
	"bytes"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/infra/webhook"
)

// An answered call returns the 2xx status and the body it carried, which an HTTP step keeps for
// later Steps (ADR 0161).
func TestExchangeReturnsTheStatusAndTheBodyOfA2xx(t *testing.T) {
	srv, allow := receiver(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, err := w.Write([]byte(`{"code":"X1"}`))
		assert.NoError(t, err)
	})

	answer, err := webhook.Exchange(t.Context(), webhook.NewClientAllowing(2*time.Second, allow), webhook.Call{URL: srv.URL, Body: []byte(`{}`), At: time.Now()})

	require.NoError(t, err)
	assert.Equal(t, webhook.Answer{Status: http.StatusCreated, Body: []byte(`{"code":"X1"}`)}, answer)
}

// answerLimit is how much of an answer is read: 64 KiB.
const answerLimit = 64 << 10

// A body exactly as long as the read limit is whole; one byte more is cut, so the step does not
// read the start of a longer document as the document.
func TestExchangeTellsABodyCutByTheLimitFromOneThatEndsThere(t *testing.T) {
	for name, tc := range map[string]struct {
		size int
		cut  bool
	}{
		"at the limit":   {answerLimit, false},
		"past the limit": {answerLimit + 1, true},
	} {
		t.Run(name, func(t *testing.T) {
			srv, allow := receiver(t, func(w http.ResponseWriter, _ *http.Request) {
				_, err := w.Write(bytes.Repeat([]byte("a"), tc.size))
				assert.NoError(t, err)
			})

			answer, err := webhook.Exchange(t.Context(), webhook.NewClientAllowing(2*time.Second, allow), webhook.Call{URL: srv.URL, Body: []byte(`{}`), At: time.Now()})

			require.NoError(t, err)
			assert.Equal(t, tc.cut, answer.Cut)
			assert.Len(t, answer.Body, answerLimit)
		})
	}
}

// A non-2xx answer is the delivery error Post returns, and no answer.
func TestExchangeReturnsNoAnswerOfANon2xx(t *testing.T) {
	srv, allow := receiver(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) })

	answer, err := webhook.Exchange(t.Context(), webhook.NewClientAllowing(2*time.Second, allow), webhook.Call{URL: srv.URL, Body: []byte(`{}`), At: time.Now()})

	require.ErrorContains(t, err, "502")
	assert.Zero(t, answer)
}
