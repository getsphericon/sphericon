// Package addresscheck judges an email address before a Contact intake brings it into a
// Workspace's audience (ADR 0117): whether its domain accepts mail at all, read from DNS, and
// whether the address is a disposable one or a role's, read from the lists of
// github.com/AfterShip/email-verifier. Only those lists are taken from the library: its MX check
// has no context, no implicit MX and no null MX, and its SMTP probe would connect to the
// recipient's server from the Instance.
package addresscheck

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	emailverifier "github.com/AfterShip/email-verifier"

	"github.com/getsphericon/sphericon/internal/kernel/domainname"
)

// Resolver is the DNS an address's domain is read from. *net.Resolver satisfies it.
type Resolver interface {
	LookupMX(ctx context.Context, name string) ([]*net.MX, error)
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// Verdict is what DNS says of a domain's mail.
type Verdict int

const (
	// Unknown is a domain DNS gave no definite answer for (a timeout, SERVFAIL, the budget spent,
	// a name that is not a registrable domain): it is accepted, as nothing proves it wrong.
	Unknown Verdict = iota
	// Accepts is a domain with a mail exchanger: an MX record, or an address record that serves
	// as its implicit MX (RFC 5321 section 5.1).
	Accepts
	// NoMail is a domain that cannot receive mail: it does not exist, it has neither an MX nor an
	// address record, or it publishes a null MX (RFC 7505).
	NoMail
)

// Flag is a trait of an address that does not keep it out of the audience but is worth the
// importer's attention.
type Flag string

const (
	// Disposable is an address at a throwaway mailbox provider.
	Disposable Flag = "disposable"
	// Role is an address of a function rather than a person (info@, sales@).
	Role Flag = "role"
)

// lookupBudget bounds every lookup of one batch together: a domain still unanswered when it is
// spent is Unknown, so a slow resolver delays an intake by at most this much.
const lookupBudget = 5 * time.Second

// lookupWorkers bounds the lookups in flight at once.
const lookupWorkers = 16

// lists is the library's verifier, used only for its lists (no network: auto-update, SMTP and
// Gravatar stay off).
var lists = emailverifier.NewVerifier()

// Domain is the domain of an email address in the form DNS reads it (lower-case punycode), or ""
// when the address has none that is a registrable domain.
func Domain(address string) string {
	_, domain, ok := strings.CutLast(address, "@")
	if !ok {
		return ""
	}
	d, ok := domainname.Normalize(domain)
	if !ok {
		return ""
	}
	return d
}

// Flags are the traits of address worth the importer's attention, in a fixed order.
func Flags(address string) []Flag {
	local, _, ok := strings.CutLast(address, "@")
	if !ok {
		return nil
	}
	var out []Flag
	if d := Domain(address); d != "" && lists.IsDisposable(d) {
		out = append(out, Disposable)
	}
	if lists.IsRoleAccount(local) {
		out = append(out, Role)
	}
	return out
}

// Domains reads the Verdict of each domain, at most lookupWorkers at a time and within
// lookupBudget for all of them. A domain missing from the result, or with no resolver, is Unknown.
func Domains(ctx context.Context, r Resolver, domains []string) map[string]Verdict {
	out := make(map[string]Verdict, len(domains))
	if r == nil || len(domains) == 0 {
		return out
	}
	ctx, cancel := context.WithTimeout(ctx, lookupBudget)
	defer cancel()
	var (
		mu   sync.Mutex
		wg   sync.WaitGroup
		slot = make(chan struct{}, lookupWorkers)
	)
	for _, d := range domains {
		wg.Go(func() {
			slot <- struct{}{}
			defer func() { <-slot }()
			v := lookup(ctx, r, d)
			mu.Lock()
			out[d] = v
			mu.Unlock()
		})
	}
	wg.Wait()
	return out
}

// lookup reads one domain: its MX records, and, when it has none, its address records.
func lookup(ctx context.Context, r Resolver, domain string) Verdict {
	if ctx.Err() != nil {
		return Unknown
	}
	mx, err := r.LookupMX(ctx, domain)
	switch {
	case err == nil && len(mx) > 0:
		if len(mx) == 1 && strings.TrimSuffix(mx[0].Host, ".") == "" {
			return NoMail
		}
		return Accepts
	case err != nil && !notFound(err):
		return Unknown
	}
	addrs, err := r.LookupNetIP(ctx, "ip", domain)
	switch {
	case err == nil && len(addrs) > 0:
		return Accepts
	case err == nil || notFound(err):
		return NoMail
	default:
		return Unknown
	}
}

// notFound is DNS's definite "no such record": NXDOMAIN or a name without records of the type.
// A timeout or a server failure is not.
func notFound(err error) bool {
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr) && dnsErr.IsNotFound && !dnsErr.IsTimeout && !dnsErr.IsTemporary
}

// AllAccept is the Resolver of an Instance whose addresses are at no real domain (the e2e suite,
// the test harness): every domain has a mail exchanger.
type AllAccept struct{}

// LookupMX answers every domain with one mail exchanger.
func (AllAccept) LookupMX(_ context.Context, name string) ([]*net.MX, error) {
	return []*net.MX{{Host: "mx." + name + ".", Pref: 10}}, nil
}

// LookupNetIP is never reached: every domain has an MX.
func (AllAccept) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return nil, nil
}
