package addresscheck_test

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"

	"github.com/foxcpp/go-mockdns"
	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/dnstest"
	"github.com/getsphericon/sphericon/internal/kernel/addresscheck"
)

func TestDomainsReadsWhetherADomainAcceptsMail(t *testing.T) {
	r := dnstest.Resolver(t, map[string]mockdns.Zone{
		"mx.test.":        {MX: []net.MX{{Host: "mail.mx.test.", Pref: 10}}},
		"implicit.test.":  {A: []string{"192.0.2.10"}},
		"implicit6.test.": {AAAA: []string{"2001:db8::10"}},
		"nullmx.test.":    {MX: []net.MX{{Host: ".", Pref: 0}}, A: []string{"192.0.2.11"}},
		"empty.test.":     {TXT: []string{"v=spf1 -all"}},
		"broken.test.":    {Err: errors.New("server failure")},
	})

	got := addresscheck.Domains(context.Background(), r, []string{
		"mx.test", "implicit.test", "implicit6.test", "nullmx.test", "empty.test", "missing.test", "broken.test",
	})

	assert.Equal(t, map[string]addresscheck.Verdict{
		"mx.test":        addresscheck.Accepts,
		"implicit.test":  addresscheck.Accepts,
		"implicit6.test": addresscheck.Accepts,
		"nullmx.test":    addresscheck.NoMail,
		"empty.test":     addresscheck.NoMail,
		"missing.test":   addresscheck.NoMail,
		"broken.test":    addresscheck.Unknown,
	}, got)
}

func TestDomainsWithoutAResolverKnowsNothing(t *testing.T) {
	assert.Empty(t, addresscheck.Domains(context.Background(), nil, []string{"example.com"}))
}

func TestDomainsOnASpentContextKnowsNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got := addresscheck.Domains(ctx, unreachable{}, []string{"a.test", "b.test"})

	assert.Equal(t, map[string]addresscheck.Verdict{"a.test": addresscheck.Unknown, "b.test": addresscheck.Unknown}, got)
}

func TestDomainIsTheRegistrableDomainOfAnAddress(t *testing.T) {
	assert.Equal(t, "example.com", addresscheck.Domain("Ann@Example.COM"))
	assert.Equal(t, "xn--bcher-kva.example", addresscheck.Domain("ann@bücher.example"))
	assert.Empty(t, addresscheck.Domain("ann@localhost"))
	assert.Empty(t, addresscheck.Domain("no-at-sign"))
}

func TestFlagsNameDisposableAndRoleAddresses(t *testing.T) {
	assert.Equal(t, []addresscheck.Flag{addresscheck.Disposable}, addresscheck.Flags("ann@mailinator.com"))
	assert.Equal(t, []addresscheck.Flag{addresscheck.Role}, addresscheck.Flags("info@example.com"))
	assert.Equal(t, []addresscheck.Flag{addresscheck.Disposable, addresscheck.Role}, addresscheck.Flags("sales@mailinator.com"))
	assert.Empty(t, addresscheck.Flags("ann@gmail.com"))
}

// unreachable is a resolver no lookup may reach.
type unreachable struct{}

func (unreachable) LookupMX(context.Context, string) ([]*net.MX, error) {
	panic("a spent context must not look up")
}

func (unreachable) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	panic("a spent context must not look up")
}
