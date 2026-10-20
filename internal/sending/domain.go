package sending

import (
	"net"
	"strings"

	"golang.org/x/net/idna"
)

// NormalizeDomain validates raw as a registrable domain name (IDNA registration rules:
// label syntax, length limits, punycode) with at least one dot and not an IP address, and
// returns its lower-case ASCII (punycode) form, the form DNS records and stored domains
// use. ok is false for anything else.
func NormalizeDomain(raw string) (string, bool) {
	d := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), ".")
	if !strings.Contains(d, ".") || net.ParseIP(d) != nil {
		return "", false
	}
	ascii, err := idna.Registration.ToASCII(d)
	if err != nil {
		return "", false
	}
	return ascii, true
}
