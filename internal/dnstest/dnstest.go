// Package dnstest runs an in-process DNS server for tests, so code that resolves
// names goes through a real *net.Resolver (NXDOMAIN, SERVFAIL, multi-string TXT)
// instead of a hand-built stub.
package dnstest

import (
	"context"
	"net"
	"testing"

	"github.com/foxcpp/go-mockdns"
)

// Resolver serves zones (keys are fully qualified, with the trailing dot) and
// returns a *net.Resolver wired to the server; the server stops with the test.
// A name outside every zone answers NXDOMAIN; a zone with Err set answers SERVFAIL.
func Resolver(tb testing.TB, zones map[string]mockdns.Zone) *net.Resolver {
	tb.Helper()
	srv, err := mockdns.NewServerWithLogger(zones, logger{tb}, false)
	if err != nil {
		tb.Fatalf("start mock DNS server: %v", err)
	}
	tb.Cleanup(func() { _ = srv.Close() })
	return srv.NewResolver()
}

// typeTXT is the DNS record type number of TXT (RFC 1035).
const typeTXT uint16 = 16

// Zone is an in-process DNS server whose TXT records a test edits while it runs, so a
// test publishes and withdraws a record (a Join domain's proof) between two calls of the
// code under test. Each Zone is its own server, so tests never share records.
type Zone struct {
	srv *mockdns.Server
	r   *net.Resolver
}

// NewZone starts an empty zone: every name answers NXDOMAIN until a record is published.
// The server stops with the test.
func NewZone(tb testing.TB) *Zone {
	tb.Helper()
	srv, err := mockdns.NewServerWithLogger(map[string]mockdns.Zone{}, logger{tb}, false)
	if err != nil {
		tb.Fatalf("start mock DNS server: %v", err)
	}
	tb.Cleanup(func() { _ = srv.Close() })
	return &Zone{srv: srv, r: srv.NewResolver()}
}

// PublishTXT adds a TXT record with value at name (no trailing dot needed).
func (z *Zone) PublishTXT(tb testing.TB, name, value string) {
	tb.Helper()
	if err := z.srv.AppendRR(name, typeTXT, value); err != nil {
		tb.Fatalf("publish TXT %s: %v", name, err)
	}
}

// WithdrawTXT removes every TXT record at name.
func (z *Zone) WithdrawTXT(name string) { z.srv.RemoveRR(name, typeTXT) }

// LookupTXT resolves through the zone's server; it satisfies sending.TXTLookup.
func (z *Zone) LookupTXT(ctx context.Context, name string) ([]string, error) {
	return z.r.LookupTXT(ctx, name)
}

// logger routes the server's own log lines to the test log.
type logger struct{ t testing.TB }

func (l logger) Printf(format string, args ...any) { l.t.Logf(format, args...) }
