package sending

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// LookupDMARC reports whether a DMARC record is published for the domain, found the way a
// receiver finds it (RFC 7489 §6.6.3): at _dmarc.<domain> and, when that name has no DMARC
// record, at _dmarc.<organizational domain>. A record is found when exactly one TXT record at
// the name starts with v=DMARC1; any policy counts, p=none too (ADR 0012). Two or more end the
// discovery with no record, as receivers ignore them. A missing record is false with a nil error;
// a resolver failure returns the error, so the caller keeps what it knew.
func LookupDMARC(ctx context.Context, lookup TXTLookup, domain string) (bool, error) {
	n, err := dmarcRecords(ctx, lookup, domain)
	if err != nil || n > 0 {
		return n == 1, err
	}
	org, ok := organizationalDomain(domain)
	if !ok {
		return false, nil
	}
	n, err = dmarcRecords(ctx, lookup, org)
	return n == 1, err
}

// organizationalDomain is the domain's organizational domain (RFC 7489 §3.2, from the public
// suffix list), and false when there is none other to ask: the domain is its own, or the list
// does not place it.
func organizationalDomain(domain string) (string, bool) {
	org, err := publicsuffix.EffectiveTLDPlusOne(domain)
	return org, err == nil && org != domain
}

// dmarcRecords counts the DMARC records at _dmarc.<domain>; NXDOMAIN is none.
func dmarcRecords(ctx context.Context, lookup TXTLookup, domain string) (int, error) {
	host, _ := DMARCRecord(domain)
	// Fully qualified, for the reason VerifyDKIM gives.
	records, err := lookup(ctx, host+".")
	if err != nil {
		if dnsIsNotFound(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("lookup %s: %w", host, err)
	}
	n := 0
	for _, rec := range records {
		if isDMARC(rec) {
			n++
		}
	}
	return n, nil
}

// isDMARC reports whether a TXT record is a DMARC record: its first tag is v=DMARC1.
func isDMARC(txt string) bool {
	first, _, _ := strings.Cut(txt, ";")
	name, value, ok := strings.Cut(first, "=")
	return ok && strings.TrimSpace(name) == "v" && strings.TrimSpace(value) == "DMARC1"
}
