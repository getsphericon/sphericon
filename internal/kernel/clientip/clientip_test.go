package clientip_test

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/kernel/clientip"
)

// The default of SPHERICON_TRUSTED_PROXY_CIDRS: loopback and the private networks.
var trusted = []netip.Prefix{
	netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"), netip.MustParsePrefix("fc00::/7"),
}

func request(t *testing.T, remote, xff string) *http.Request {
	t.Helper()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	r.RemoteAddr = remote
	if xff != "" {
		r.Header.Set("X-Forwarded-For", xff)
	}
	return r
}

// ADR 0175: X-Forwarded-For is read only from a trusted proxy, and the rightmost address no trusted
// proxy holds is the client's.
func TestTheClientAddressIsReadBehindTrustedProxiesOnly(t *testing.T) {
	c := clientip.New(trusted)
	for name, tc := range map[string]struct{ remote, xff, want string }{
		"a client with no proxy":                    {"203.0.113.9:4000", "", "203.0.113.9"},
		"a client that reaches the binary directly": {"203.0.113.9:4000", "6.6.6.6", "203.0.113.9"},
		"a client behind the proxy":                 {"10.0.0.1:4000", "198.51.100.7", "198.51.100.7"},
		"a spoofed entry before the proxy's own":    {"10.0.0.1:4000", "6.6.6.6, 198.51.100.7", "198.51.100.7"},
		"two proxies in front":                      {"10.0.0.1:4000", "198.51.100.7, 10.0.0.2", "198.51.100.7"},
		"a proxy that forwards no header":           {"127.0.0.1:4000", "", "127.0.0.1"},
		"a header of trusted addresses only":        {"10.0.0.1:4000", "10.0.0.2", "10.0.0.1"},
		"an address with no port":                   {"203.0.113.9", "", "203.0.113.9"},
		"an IPv6 client behind an IPv6 proxy":       {"[::1]:4000", "2001:db8::7", "2001:db8::7"},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, c.FromRequest(request(t, tc.remote, tc.xff)))
		})
	}
}

// With no trusted proxy the connection's own address is the client's, whatever the header says.
func TestNoTrustedProxyIgnoresTheHeader(t *testing.T) {
	assert.Equal(t, "10.0.0.1", clientip.New(nil).FromRequest(request(t, "10.0.0.1:4000", "198.51.100.7")))
}

func TestMiddlewareExposesAddressToContext(t *testing.T) {
	var got, gotUA string
	h := clientip.New(trusted).Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = clientip.FromContext(r.Context())
		gotUA = clientip.UserAgentFromContext(r.Context())
	}))
	r := request(t, "10.0.0.9:5555", "")
	r.Header.Set("User-Agent", "test-agent/1.0")
	h.ServeHTTP(httptest.NewRecorder(), r)
	assert.Equal(t, "10.0.0.9", got)
	assert.Equal(t, "test-agent/1.0", gotUA)
	assert.Empty(t, clientip.FromContext(t.Context()))
	assert.Empty(t, clientip.UserAgentFromContext(t.Context()))
}
