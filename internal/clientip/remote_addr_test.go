package clientip_test

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/clientip"
)

func TestFromRequestUsesTheRawRemoteAddrWhenItHasNoPort(t *testing.T) {
	r := httptest.NewRequestWithContext(t.Context(), "GET", "/", nil)
	r.RemoteAddr = "203.0.113.9"
	assert.Equal(t, "203.0.113.9", clientip.FromRequest(r))
}

// Only the hop our own proxy appended is trusted: a client-supplied leading entry
// cannot spoof the recorded address.
func TestFromRequestTrustsOnlyTheHopTheProxyAppended(t *testing.T) {
	r := httptest.NewRequestWithContext(t.Context(), "GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:4000"
	r.Header.Set("X-Forwarded-For", "6.6.6.6, 198.51.100.7")
	assert.Equal(t, "198.51.100.7", clientip.FromRequest(r))
}
