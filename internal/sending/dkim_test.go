package sending_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/foxcpp/go-mockdns"
	"github.com/getsphericon/sphericon/internal/dnstest"
	"github.com/getsphericon/sphericon/internal/sending"
)

func TestGenerateKeypairRoundTrip(t *testing.T) {
	privPEM, pubTXT, err := sending.GenerateKeypair()
	require.NoErrorf(t, err, "GenerateKeypair: %v", err)
	require.Truef(t, strings.HasPrefix(pubTXT, "v=DKIM1; k=rsa; p="), "unexpected TXT value: %q", pubTXT)
	require.Containsf(t, string(privPEM), "PRIVATE KEY", "private PEM missing key block: %q", privPEM)

	// The public key re-derived from the stored private PEM must match the TXT
	// value handed to the user — this is exactly what verification compares.
	derived, err := sending.PublicKeyFromPrivatePEM(privPEM)
	require.NoErrorf(t, err, "PublicKeyFromPrivatePEM: %v", err)
	require.Equalf(t, pubTXT, derived, "derived public key mismatch:\n got %q\nwant %q", derived, pubTXT)
}

func TestDKIMRecordHost(t *testing.T) {
	host, value := sending.DKIMRecord("sphericon", "mail.acme.com", "v=DKIM1; k=rsa; p=abc")
	require.Equalf(t, "sphericon._domainkey.mail.acme.com", host, "host = %q", host)
	require.Equalf(t, "v=DKIM1; k=rsa; p=abc", value, "value = %q", value)
}

const (
	dkimZone = "sphericon._domainkey.mail.acme.com."
	dkimPub  = "v=DKIM1; k=rsa; p=MIIBIjANBgkqABC"
)

func TestVerifyDKIM(t *testing.T) {
	tests := []struct {
		name  string
		zones map[string]mockdns.Zone
		want  bool
	}{
		{
			name:  "exact match",
			zones: map[string]mockdns.Zone{dkimZone: {TXT: []string{dkimPub}}},
			want:  true,
		},
		{
			name:  "match among several records",
			zones: map[string]mockdns.Zone{dkimZone: {TXT: []string{"v=spf1 -all", dkimPub}}},
			want:  true,
		},
		{
			name:  "match despite interior whitespace in p=",
			zones: map[string]mockdns.Zone{dkimZone: {TXT: []string{"v=DKIM1; k=rsa; p=MIIBIjANBg kqABC"}}},
			want:  true,
		},
		{
			name:  "wrong key",
			zones: map[string]mockdns.Zone{dkimZone: {TXT: []string{"v=DKIM1; k=rsa; p=DIFFERENT"}}},
			want:  false,
		},
		{
			name:  "no record published (NXDOMAIN)",
			zones: map[string]mockdns.Zone{},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookup := dnstest.Resolver(t, tt.zones).LookupTXT
			got, err := sending.VerifyDKIM(context.Background(), lookup, "sphericon", "mail.acme.com", dkimPub)
			require.NoError(t, err)
			require.Equalf(t, tt.want, got, "got %v, want %v", got, tt.want)
		})
	}
}

// A resolver failure is not "no record": it surfaces, so the check is retried rather than
// the domain reported unverified.
func TestVerifyDKIMSurfacesAResolverFailure(t *testing.T) {
	lookup := dnstest.Resolver(t, map[string]mockdns.Zone{dkimZone: {Err: errors.New("server misbehaving")}}).LookupTXT

	got, err := sending.VerifyDKIM(context.Background(), lookup, "sphericon", "mail.acme.com", dkimPub)

	require.Error(t, err)
	require.False(t, got)
}
