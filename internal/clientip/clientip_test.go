package clientip_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getsphericon/sphericon/internal/clientip"
	"github.com/stretchr/testify/assert"
)

func TestFromRequest(t *testing.T) {
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.9:5555"
	assert.Equal(t, "10.0.0.9", clientip.FromRequest(r), "remote host without a proxy header")

	r.Header.Set("X-Forwarded-For", "203.0.113.7")
	assert.Equal(t, "203.0.113.7", clientip.FromRequest(r), "the hop our proxy appended wins")
}

func TestMiddlewareExposesAddressToContext(t *testing.T) {
	var got, gotUA string
	h := clientip.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = clientip.FromContext(r.Context())
		gotUA = clientip.UserAgentFromContext(r.Context())
	}))
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.9:5555"
	r.Header.Set("User-Agent", "test-agent/1.0")
	h.ServeHTTP(httptest.NewRecorder(), r)
	assert.Equal(t, "10.0.0.9", got)
	assert.Equal(t, "test-agent/1.0", gotUA)
	assert.Empty(t, clientip.FromContext(t.Context()))
	assert.Empty(t, clientip.UserAgentFromContext(t.Context()))
}
