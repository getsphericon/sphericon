package webhook

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Allowlist names the internal host:port pairs an outbound call may reach although the
// address is private or loopback (ADR 0039). It is an instance-level setting, empty by
// default and never set in SaaS; there is no per-Workspace form. The zero value allows
// nothing.
type Allowlist struct {
	entries map[string]struct{}
}

// ParseAllowlist reads a comma- or space-separated list of host:port entries (a hostname,
// an IPv4 address or a bracketed IPv6 address, then a port). An empty value is the empty
// allowlist; a malformed entry is an error naming it.
func ParseAllowlist(value string) (Allowlist, error) {
	fields := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || unicode.IsSpace(r) })
	allow := Allowlist{entries: make(map[string]struct{}, len(fields))}
	for _, field := range fields {
		host, port, err := net.SplitHostPort(field)
		if err != nil {
			return Allowlist{}, fmt.Errorf("%q is not host:port: %w", field, err)
		}
		if host == "" || strings.ContainsAny(host, "*/%") {
			return Allowlist{}, fmt.Errorf("%q: the host must be a name or an IP address, with no wildcard or range", field)
		}
		if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
			return Allowlist{}, fmt.Errorf("%q: %q is not a valid port", field, port)
		}
		allow.entries[allowKey(host, port)] = struct{}{}
	}
	return allow, nil
}

func allowKey(host, port string) string { return strings.ToLower(host) + ":" + port }

// NewClientAllowing is NewClient for the callers that honour the Instance's internal
// allowlist (ADR 0039): a request to a host:port it names is made without the address
// check, every other request goes through the hardened client. Redirects are never
// followed either way, so a permitted host cannot bounce a call to an address that is not.
func NewClientAllowing(timeout time.Duration, allow Allowlist) Doer {
	hardened := NewClient(timeout)
	if len(allow.entries) == 0 {
		return hardened
	}
	return allowingDoer{allow: allow, hardened: hardened, plain: &http.Client{
		Timeout:       timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

// allowingDoer sends a request to an allowlisted host:port through the plain client and any
// other through the hardened one.
type allowingDoer struct {
	allow    Allowlist
	hardened Doer
	plain    Doer
}

func (d allowingDoer) Do(req *http.Request) (*http.Response, error) {
	if _, ok := d.allow.entries[allowKey(req.URL.Hostname(), portOf(req.URL))]; ok {
		return d.plain.Do(req)
	}
	return d.hardened.Do(req)
}

func portOf(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}
