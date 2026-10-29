package sending_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/sending"
)

func TestSPFAndDMARCRecords(t *testing.T) {
	host, value := sending.SPFRecord("mail.acme.com")
	require.Equal(t, "mail.acme.com", host)
	require.Equal(t, "v=spf1 include:amazonses.com ~all", value)

	host, value = sending.DMARCRecord("mail.acme.com")
	require.Equal(t, "_dmarc.mail.acme.com", host)
	require.Equal(t, "v=DMARC1; p=none;", value)
}

func TestPublicKeyFromPrivatePEMRejectsBadKeys(t *testing.T) {
	_, edKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	edDER, err := x509.MarshalPKCS8PrivateKey(edKey)
	require.NoError(t, err)

	cases := map[string]struct {
		pem  []byte
		want string
	}{
		"no pem block":   {[]byte("not a pem"), "no PEM block"},
		"bad der":        {pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("junk")}), "parse private key"},
		"not an rsa key": {pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: edDER}), "not RSA"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := sending.PublicKeyFromPrivatePEM(tc.pem)
			require.ErrorContains(t, err, tc.want)
			require.Emptyf(t, got, "got %q, want empty", got)
		})
	}
}

func TestVerifyDKIMRejectsEmptyExpectedKey(t *testing.T) {
	lookup := func(context.Context, string) ([]string, error) { return []string{"v=DKIM1; k=rsa; p=abc"}, nil }
	ok, err := sending.VerifyDKIM(context.Background(), lookup, "sphericon", "acme.com", "v=DKIM1; k=rsa; p=")
	require.False(t, ok, "an empty expected key never verifies")
	require.Error(t, err)
}
