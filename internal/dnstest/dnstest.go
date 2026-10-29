// Package dnstest runs an in-process DNS server for tests, so code that resolves
// names goes through a real *net.Resolver (NXDOMAIN, SERVFAIL, multi-string TXT)
// instead of a hand-built stub.
package dnstest

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/foxcpp/go-mockdns"
	"github.com/miekg/dns"

	"github.com/getsphericon/sphericon/internal/cleanup"
)

// Resolver serves zones (keys are fully qualified, with the trailing dot) and
// returns a *net.Resolver wired to the server; the server stops with the test.
// A name outside every zone answers NXDOMAIN; a zone with Err set answers SERVFAIL.
func Resolver(tb testing.TB, zones map[string]mockdns.Zone) *net.Resolver {
	tb.Helper()
	return start(tb, zones).NewResolver()
}

// Addr serves zones like Resolver and returns the server's address, for code that is configured
// with a DNS server rather than handed a resolver.
func Addr(tb testing.TB, zones map[string]mockdns.Zone) string {
	tb.Helper()
	return start(tb, zones).LocalAddr().String()
}

// start serves zones until the test ends. go-mockdns starts its UDP and TCP servers in goroutines,
// and a Close that comes before one of them has started stops nothing: that server then serves
// for the rest of the process, and goleak fails the package. So start returns only once the
// server has answered over both.
func start(tb testing.TB, zones map[string]mockdns.Zone) *mockdns.Server {
	tb.Helper()
	srv, err := mockdns.NewServerWithLogger(zones, logger{tb}, false)
	if err != nil {
		tb.Fatalf("start mock DNS server: %v", err)
	}
	tb.Cleanup(func() { cleanup.Report("stop the mock DNS server", srv.Close()) })
	probe := new(dns.Msg)
	probe.SetQuestion("ready.dnstest.", dns.TypeA)
	for _, network := range []string{"udp", "tcp"} {
		client := &dns.Client{Net: network, Timeout: 10 * time.Second}
		if _, _, err := client.Exchange(probe, srv.LocalAddr().String()); err != nil {
			tb.Fatalf("mock DNS server does not answer over %s: %v", network, err)
		}
	}
	return srv
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
	srv := start(tb, map[string]mockdns.Zone{})
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
