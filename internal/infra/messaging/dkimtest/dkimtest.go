// Package dkimtest gives a provider test a Sending domain's DKIM key without a database: a
// messaging.DKIMKeys for one domain, with an ed25519 key made when the test runs, and the check
// that a message the provider sent carries a signature that verifies against that key's public
// record.
package dkimtest

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/emersion/go-msgauth/dkim"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/infra/messaging"
)

// selector is the selector every Keys publishes its record under.
const selector = "sphericon"

// Keys is the DKIM key of one verified Sending domain.
type Keys struct {
	domain string
	public ed25519.PublicKey
	key    ed25519.PrivateKey
}

// New makes the key of domain.
func New(t *testing.T, domain string) *Keys {
	t.Helper()
	public, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return &Keys{domain: domain, public: public, key: key}
}

// Key returns the key when fromEmail is an address of the domain, and nil otherwise, as an
// unverified domain does (messaging.DKIMKeys).
func (k *Keys) Key(_ context.Context, fromEmail string) (*messaging.DKIMKey, error) {
	domain := messaging.DomainOf(fromEmail)
	if domain != k.domain {
		return nil, nil
	}
	return &messaging.DKIMKey{Domain: k.domain, Selector: selector, Signer: k.key}, nil
}

// Verify checks that raw carries exactly one DKIM signature, made by this domain, that
// verifies against the domain's public record, and returns the header fields it signs.
func (k *Keys) Verify(t *testing.T, raw []byte) (signedHeaders []string) {
	t.Helper()
	record := "v=DKIM1; k=ed25519; p=" + base64.StdEncoding.EncodeToString(k.public)
	recordName := selector + "._domainkey." + k.domain
	verifs, err := dkim.VerifyWithOptions(bytes.NewReader(raw), &dkim.VerifyOptions{
		LookupTXT: func(name string) ([]string, error) {
			if name != recordName {
				return nil, fmt.Errorf("dkimtest: no record at %s", name)
			}
			return []string{record}, nil
		},
	})
	require.NoError(t, err)
	require.Len(t, verifs, 1, "one DKIM signature")
	require.NoError(t, verifs[0].Err, "the DKIM signature verifies")
	require.Equal(t, k.domain, verifs[0].Domain)
	return verifs[0].HeaderKeys
}
