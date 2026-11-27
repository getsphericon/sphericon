package sendingdomains_test

import (
	"context"
	"errors"
	"testing"

	"github.com/foxcpp/go-mockdns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/dnstest"
	"github.com/getsphericon/sphericon/internal/domain/sending/sendingdomains"
	"github.com/getsphericon/sphericon/internal/kernel/domainname"
)

const (
	dmarcSub = "_dmarc.mail.acme.com."
	dmarcOrg = "_dmarc.acme.com."
)

// The record is looked up the way a receiver does, by the DNS tree walk of RFC 9989 §4.10: at the
// domain, then at each parent until a name holds one.
func TestLookupDMARC(t *testing.T) {
	tests := []struct {
		name   string
		domain string
		zones  map[string]mockdns.Zone
		want   bool
	}{
		{
			name:   "a record at the domain",
			domain: "mail.acme.com",
			zones:  map[string]mockdns.Zone{dmarcSub: {TXT: []string{"v=DMARC1; p=none;"}}},
			want:   true,
		},
		{
			name:   "any policy counts, the strict ones too",
			domain: "mail.acme.com",
			zones:  map[string]mockdns.Zone{dmarcSub: {TXT: []string{"v=DMARC1; p=reject; rua=mailto:d@acme.com"}}},
			want:   true,
		},
		{
			name:   "a record only at the organizational domain",
			domain: "mail.acme.com",
			zones:  map[string]mockdns.Zone{dmarcOrg: {TXT: []string{"v=DMARC1; p=none;"}}},
			want:   true,
		},
		{
			name:   "a record only at a parent below the registered domain",
			domain: "a.b.example.com",
			zones:  map[string]mockdns.Zone{"_dmarc.b.example.com.": {TXT: []string{"v=DMARC1; p=none;"}}},
			want:   true,
		},
		{
			name:   "an organizational domain is its own",
			domain: "acme.com",
			zones:  map[string]mockdns.Zone{dmarcOrg: {TXT: []string{"v=DMARC1; p=none;"}}},
			want:   true,
		},
		{
			name:   "other TXT records at the name are not DMARC",
			domain: "mail.acme.com",
			zones:  map[string]mockdns.Zone{dmarcSub: {TXT: []string{"v=spf1 -all", "v=DMARC10; p=none;"}}},
			want:   false,
		},
		{
			name:   "no record anywhere",
			domain: "mail.acme.com",
			zones:  map[string]mockdns.Zone{},
			want:   false,
		},
		{
			name:   "two records at the domain are ignored, as receivers ignore them",
			domain: "mail.acme.com",
			zones:  map[string]mockdns.Zone{dmarcSub: {TXT: []string{"v=DMARC1; p=none;", "v=DMARC1; p=reject;"}}},
			want:   false,
		},
		{
			name:   "two records at the domain are discarded and the walk goes on to the parent",
			domain: "mail.acme.com",
			zones: map[string]mockdns.Zone{
				dmarcSub: {TXT: []string{"v=DMARC1; p=none;", "v=DMARC1; p=reject;"}},
				dmarcOrg: {TXT: []string{"v=DMARC1; p=none;"}},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookup := dnstest.Resolver(t, tt.zones).LookupTXT

			got, err := sendingdomains.NewDNS(lookup).LookupDMARC(t.Context(), tt.domain)

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

// The walk asks at most eight names: a long domain skips to its right-most seven labels and goes on
// to the top-level domain. The names are RFC 9989 §4.10's own example.
func TestLookupDMARCWalksAtMostEightNames(t *testing.T) {
	var asked []string
	lookup := func(_ context.Context, name string) ([]string, error) {
		asked = append(asked, name)
		return nil, nil
	}

	got, err := sendingdomains.NewDNS(lookup).LookupDMARC(t.Context(), "a.b.c.d.e.f.g.h.i.j.mail.example.com")

	require.NoError(t, err)
	assert.False(t, got)
	assert.Equal(t, []string{
		"_dmarc.a.b.c.d.e.f.g.h.i.j.mail.example.com.",
		"_dmarc.g.h.i.j.mail.example.com.",
		"_dmarc.h.i.j.mail.example.com.",
		"_dmarc.i.j.mail.example.com.",
		"_dmarc.j.mail.example.com.",
		"_dmarc.mail.example.com.",
		"_dmarc.example.com.",
		"_dmarc.com.",
	}, asked)
}

// A record at the domain itself ends the walk: no parent is asked.
func TestLookupDMARCStopsAtTheFirstRecord(t *testing.T) {
	var asked []string
	lookup := func(_ context.Context, name string) ([]string, error) {
		asked = append(asked, name)
		return []string{"v=DMARC1; p=none;"}, nil
	}

	got, err := sendingdomains.NewDNS(lookup).LookupDMARC(t.Context(), "mail.acme.com")

	require.NoError(t, err)
	assert.True(t, got)
	assert.Equal(t, []string{dmarcSub}, asked)
}

// FuzzLookupDMARC states what holds for any TXT record a DNS server returns: the lookup never
// panics nor errs on an answer, a record behind a v=DMARC1 tag is found, the same record twice is
// never found, and a TXT record that is not DMARC beside it changes nothing.
func FuzzLookupDMARC(f *testing.F) {
	for _, seed := range []string{
		"v=DMARC1; p=none;", "v=DMARC1;p=reject;rua=mailto:d@acme.com", " v = DMARC1 ; p=none",
		"v=DMARC10; p=none;", "v=spf1 -all", "p=none; v=DMARC1", "", ";", "=", "v=",
	} {
		f.Add(seed)
	}
	answer := func(records ...string) domainname.TXTLookup {
		return func(context.Context, string) ([]string, error) { return records, nil }
	}
	f.Fuzz(func(t *testing.T, record string) {
		alone, err := sendingdomains.NewDNS(answer(record)).LookupDMARC(t.Context(), "mail.acme.com")
		require.NoError(t, err)
		withSPF, err := sendingdomains.NewDNS(answer(record, "v=spf1 -all")).LookupDMARC(t.Context(), "mail.acme.com")
		require.NoError(t, err)
		twice, err := sendingdomains.NewDNS(answer(record, record)).LookupDMARC(t.Context(), "mail.acme.com")
		require.NoError(t, err)
		tagged, err := sendingdomains.NewDNS(answer("v=DMARC1; "+record)).LookupDMARC(t.Context(), "mail.acme.com")
		require.NoError(t, err)

		assert.Equal(t, alone, withSPF, "a TXT record that is not DMARC changes nothing")
		assert.False(t, twice, "two DMARC records are none")
		assert.True(t, tagged, "a record that starts with v=DMARC1 is DMARC")
	})
}

// A resolver failure is not "no record", at the domain or at a parent the walk asks before it finds
// one: it surfaces, so the stored answer is kept.
func TestLookupDMARCSurfacesAResolverFailure(t *testing.T) {
	for name, zones := range map[string]map[string]mockdns.Zone{
		"at the domain":           {dmarcSub: {Err: errors.New("server misbehaving")}, dmarcOrg: {TXT: []string{"v=DMARC1; p=none;"}}},
		"at the parent":           {dmarcOrg: {Err: errors.New("server misbehaving")}},
		"at the top-level domain": {"_dmarc.com.": {Err: errors.New("server misbehaving")}},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := sendingdomains.NewDNS(dnstest.Resolver(t, zones).LookupTXT).LookupDMARC(t.Context(), "mail.acme.com")

			require.Error(t, err)
			require.False(t, got)
		})
	}
}
