package sendingdomains

import (
	"context"
	"fmt"
	"strings"

	"github.com/getsphericon/sphericon/internal/kernel/domainname"
)

// LookupDMARC reports whether a DMARC record is published for the domain, found the way a
// receiver finds it: by the DNS tree walk of RFC 9989 §4.10, which asks _dmarc.<domain> and then
// _dmarc.<parent> for each parent up to the top-level domain, at most eight names (dmarcTargets).
// A name holds a record when exactly one TXT record there starts with v=DMARC1; two or more are
// discarded and the walk goes on to the parent, as RFC 9989 says. The first name that holds a
// record ends the walk: any policy counts, p=none too (ADR 0012), whether it is the domain's own,
// its organizational domain's or its public suffix's. A missing record is false with a nil error;
// a resolver failure returns the error, so the caller keeps what it knew.
func (n *DNS) LookupDMARC(ctx context.Context, domain string) (bool, error) {
	for _, target := range dmarcTargets(domain) {
		found, err := dmarcRecords(ctx, n.lookup, target)
		if err != nil {
			return false, err
		}
		if found == 1 {
			return true, nil
		}
	}
	return false, nil
}

// dmarcTreeWalkLimit is the most names the tree walk asks (RFC 9989 §4.10).
const dmarcTreeWalkLimit = 8

// dmarcTargets is the names the DNS tree walk asks, in order (RFC 9989 §4.10): the domain itself,
// then each parent down to the top-level domain. A domain of more than eight labels skips to its
// right-most seven, so that no walk asks more than eight names.
func dmarcTargets(domain string) []string {
	labels := strings.Split(strings.TrimSuffix(domain, "."), ".")
	targets := []string{strings.Join(labels, ".")}
	for i := max(1, len(labels)-(dmarcTreeWalkLimit-1)); i < len(labels); i++ {
		targets = append(targets, strings.Join(labels[i:], "."))
	}
	return targets
}

// dmarcRecords counts the DMARC records at _dmarc.<domain>; NXDOMAIN is none.
func dmarcRecords(ctx context.Context, lookup domainname.TXTLookup, domain string) (int, error) {
	host, _ := DMARCRecord(domain)
	// Fully qualified, for the reason DNS.VerifyDKIM gives.
	records, err := lookup(ctx, host+".")
	if err != nil {
		if domainname.NotFound(err) {
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
