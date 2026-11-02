package sending_test

import (
	"context"
	"errors"
	"testing"

	"github.com/foxcpp/go-mockdns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/dnstest"
	"github.com/getsphericon/sphericon/internal/sending"
)

const (
	dmarcSub = "_dmarc.mail.acme.com."
	dmarcOrg = "_dmarc.acme.com."
)

// The record is looked up the way a receiver does (RFC 7489 §6.6.3): at the domain, then, when the
// domain has none, at its organizational domain.
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
			name:   "two records at the domain end the discovery: the organizational domain is not asked",
			domain: "mail.acme.com",
			zones: map[string]mockdns.Zone{
				dmarcSub: {TXT: []string{"v=DMARC1; p=none;", "v=DMARC1; p=reject;"}},
				dmarcOrg: {TXT: []string{"v=DMARC1; p=none;"}},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookup := dnstest.Resolver(t, tt.zones).LookupTXT

			got, err := sending.LookupDMARC(t.Context(), lookup, tt.domain)

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
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
	answer := func(records ...string) sending.TXTLookup {
		return func(context.Context, string) ([]string, error) { return records, nil }
	}
	f.Fuzz(func(t *testing.T, record string) {
		alone, err := sending.LookupDMARC(t.Context(), answer(record), "mail.acme.com")
		require.NoError(t, err)
		withSPF, err := sending.LookupDMARC(t.Context(), answer(record, "v=spf1 -all"), "mail.acme.com")
		require.NoError(t, err)
		twice, err := sending.LookupDMARC(t.Context(), answer(record, record), "mail.acme.com")
		require.NoError(t, err)
		tagged, err := sending.LookupDMARC(t.Context(), answer("v=DMARC1; "+record), "mail.acme.com")
		require.NoError(t, err)

		assert.Equal(t, alone, withSPF, "a TXT record that is not DMARC changes nothing")
		assert.False(t, twice, "two DMARC records are none")
		assert.True(t, tagged, "a record that starts with v=DMARC1 is DMARC")
	})
}

// A resolver failure is not "no record", at the domain or at its organizational domain: it
// surfaces, so the stored answer is kept.
func TestLookupDMARCSurfacesAResolverFailure(t *testing.T) {
	for name, zones := range map[string]map[string]mockdns.Zone{
		"at the domain":                {dmarcSub: {Err: errors.New("server misbehaving")}, dmarcOrg: {TXT: []string{"v=DMARC1; p=none;"}}},
		"at the organizational domain": {dmarcOrg: {Err: errors.New("server misbehaving")}},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := sending.LookupDMARC(t.Context(), dnstest.Resolver(t, zones).LookupTXT, "mail.acme.com")

			require.Error(t, err)
			require.False(t, got)
		})
	}
}
