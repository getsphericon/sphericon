package sending

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/foxcpp/go-mockdns"
	"github.com/getsphericon/sphericon/internal/dnstest"
)

func TestGenerateKeypairRoundTrip(t *testing.T) {
	privPEM, pubTXT, err := GenerateKeypair()
	require.NoErrorf(t, err, "GenerateKeypair: %v", err)
	require.Truef(t, strings.HasPrefix(pubTXT, "v=DKIM1; k=rsa; p="), "unexpected TXT value: %q", pubTXT)
	require.Containsf(t, string(privPEM), "PRIVATE KEY", "private PEM missing key block: %q", privPEM)

	// The public key re-derived from the stored private PEM must match the TXT
	// value handed to the user — this is exactly what verification compares.
	derived, err := PublicKeyFromPrivatePEM(privPEM)
	require.NoErrorf(t, err, "PublicKeyFromPrivatePEM: %v", err)
	require.Equalf(t, pubTXT, derived, "derived public key mismatch:\n got %q\nwant %q", derived, pubTXT)
}

func TestDKIMRecordHost(t *testing.T) {
	host, value := DKIMRecord("sphericon", "mail.acme.com", "v=DKIM1; k=rsa; p=abc")
	require.Equalf(t, "sphericon._domainkey.mail.acme.com", host, "host = %q", host)
	require.Equalf(t, "v=DKIM1; k=rsa; p=abc", value, "value = %q", value)
}

const dkimZone = "sphericon._domainkey.mail.acme.com."

func TestVerifyDKIM(t *testing.T) {
	const pub = "v=DKIM1; k=rsa; p=MIIBIjANBgkqABC"
	// A 2048-bit key outgrows one 255-byte TXT string; the server splits it and
	// net.Resolver rejoins the chunks.
	longPub := "v=DKIM1; k=rsa; p=MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA" + strings.Repeat("abcdefghij", 40)

	tests := []struct {
		name    string
		zones   map[string]mockdns.Zone
		expect  string
		want    bool
		wantErr bool
	}{
		{
			name:   "exact match",
			zones:  map[string]mockdns.Zone{dkimZone: {TXT: []string{pub}}},
			expect: pub,
			want:   true,
		},
		{
			name:   "match among several records",
			zones:  map[string]mockdns.Zone{dkimZone: {TXT: []string{"v=spf1 -all", pub}}},
			expect: pub,
			want:   true,
		},
		{
			name:   "match despite interior whitespace in p=",
			zones:  map[string]mockdns.Zone{dkimZone: {TXT: []string{"v=DKIM1; k=rsa; p=MIIBIjANBg kqABC"}}},
			expect: pub,
			want:   true,
		},
		{
			name:   "match across chunked TXT strings",
			zones:  map[string]mockdns.Zone{dkimZone: {TXT: []string{longPub}}},
			expect: longPub,
			want:   true,
		},
		{
			name:   "wrong key",
			zones:  map[string]mockdns.Zone{dkimZone: {TXT: []string{"v=DKIM1; k=rsa; p=DIFFERENT"}}},
			expect: pub,
			want:   false,
		},
		{
			name:   "no record published (NXDOMAIN)",
			zones:  map[string]mockdns.Zone{},
			expect: pub,
			want:   false,
		},
		{
			name:    "resolver failure surfaces",
			zones:   map[string]mockdns.Zone{dkimZone: {Err: errors.New("server misbehaving")}},
			expect:  pub,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookup := dnstest.Resolver(t, tt.zones).LookupTXT
			got, err := VerifyDKIM(context.Background(), lookup, "sphericon", "mail.acme.com", tt.expect)
			wantErr := map[bool]require.ErrorAssertionFunc{true: require.Error, false: require.NoError}
			wantErr[tt.wantErr](t, err, "VerifyDKIM")
			require.Equalf(t, tt.want, got, "got %v, want %v", got, tt.want)
		})
	}
}
