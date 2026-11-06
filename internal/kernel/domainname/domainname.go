// Package domainname reads a domain name as the Sending, Tracking and Join domains store it, and
// names the resolver their TXT records are read with. It is a leaf, so a module that takes a
// domain from a person (a Join domain, the domain of an email address) does not import the
// Sending domain's DKIM primitives (internal/domain/sending/sending).
package domainname

import (
	"context"
	"net"
	"strings"

	"golang.org/x/net/idna"
)

// TXTLookup resolves TXT records for a name. net.Resolver satisfies it; tests
// inject a stub.
type TXTLookup func(ctx context.Context, name string) ([]string, error)

// Normalize validates raw as a registrable domain name (IDNA registration rules:
// label syntax, length limits, punycode) with at least one dot and not an IP address, and
// returns its lower-case ASCII (punycode) form, the form DNS records and stored domains
// use. ok is false for anything else.
func Normalize(raw string) (string, bool) {
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
