// Package dnstest runs an in-process DNS server for tests, so code that resolves
// names goes through a real *net.Resolver (NXDOMAIN, SERVFAIL, multi-string TXT)
// instead of a hand-built stub.
package dnstest

import (
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

// logger routes the server's own log lines to the test log.
type logger struct{ t testing.TB }

func (l logger) Printf(format string, args ...any) { l.t.Logf(format, args...) }
