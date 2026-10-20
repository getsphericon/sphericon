// Package sending owns the workspace Sending-domain authentication primitives
// (ADR 0010): per-domain DKIM keypair generation, the DNS records a user
// publishes, and DKIM DNS verification. It is transport-independent on purpose —
// no mail is sent here; the send path (internal/messaging) consumes the keys in
// a later slice.
package sending

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
)

// dkimKeyBits is the RSA key size for DKIM. 2048 is the modern floor mailbox
// providers expect (1024 is deprecated and increasingly rejected).
const dkimKeyBits = 2048

// GenerateKeypair mints a fresh RSA keypair for a Sending domain. It returns the
// PKCS#8 PEM private key (to be Tink-encrypted before storage) and the DKIM TXT
// record value carrying the public key ("v=DKIM1; k=rsa; p=<base64 DER>").
func GenerateKeypair() (privPEM []byte, pubTXT string, err error) {
	key, err := rsa.GenerateKey(rand.Reader, dkimKeyBits)
	if err != nil {
		return nil, "", fmt.Errorf("generate rsa key: %w", err)
	}

	privDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, "", fmt.Errorf("marshal private key: %w", err)
	}
	privPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})

	pubTXT, err = publicKeyTXT(&key.PublicKey)
	if err != nil {
		return nil, "", err
	}
	return privPEM, pubTXT, nil
}

// publicKeyTXT builds the DKIM TXT record value for an RSA public key.
func publicKeyTXT(pub *rsa.PublicKey) (string, error) {
	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", fmt.Errorf("marshal public key: %w", err)
	}
	return "v=DKIM1; k=rsa; p=" + base64.StdEncoding.EncodeToString(pubDER), nil
}

// PublicKeyFromPrivatePEM re-derives the DKIM TXT value from a stored private
// key PEM — used to check a published record against our key without persisting
// the public value separately.
func PublicKeyFromPrivatePEM(privPEM []byte) (string, error) {
	block, _ := pem.Decode(privPEM)
	if block == nil {
		return "", fmt.Errorf("no PEM block in private key")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("parse private key: %w", err)
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return "", fmt.Errorf("private key is not RSA")
	}
	return publicKeyTXT(&rsaKey.PublicKey)
}

// DKIMRecord returns the DNS host and value the user publishes to authenticate
// the domain. host is "<selector>._domainkey.<domain>".
func DKIMRecord(selector, domain, pubTXT string) (host, value string) {
	return fmt.Sprintf("%s._domainkey.%s", selector, domain), pubTXT
}

// SPFRecord returns the suggested SPF TXT for the domain. Advisory only — native
// DKIM signing deliberately abstracts the sending IPs, so SPF does not gate
// (ADR 0010). host is the domain apex.
func SPFRecord(domain string) (host, value string) {
	return domain, "v=spf1 include:amazonses.com ~all"
}

// DMARCRecord returns the suggested _dmarc TXT. Advisory bulk-readiness signal
// (ADR 0012): Gmail/Yahoo require at least p=none for bulk senders, but sphericon
// never mandates the stricter policies, which live on the organizational domain.
func DMARCRecord(domain string) (host, value string) {
	return "_dmarc." + domain, "v=DMARC1; p=none;"
}

// TXTLookup resolves TXT records for a name. net.Resolver satisfies it; tests
// inject a stub.
type TXTLookup func(ctx context.Context, name string) ([]string, error)

// VerifyDKIM reports whether the DKIM TXT published at "<selector>._domainkey.
// <domain>" carries the same public key as expectedPubTXT. It is tolerant of TXT
// chunking (records split into multiple strings) and whitespace differences in
// the p= tag. A missing record verifies false with a nil error; a resolver
// failure returns the error.
func VerifyDKIM(ctx context.Context, lookup TXTLookup, selector, domain, expectedPubTXT string) (bool, error) {
	host, _ := DKIMRecord(selector, domain, expectedPubTXT)
	// Fully qualified: a relative name would be retried through the host's resolv.conf
	// search suffixes, which can turn a SERVFAIL into a misleading "not found".
	records, err := lookup(ctx, host+".")
	if err != nil {
		if dnsIsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("lookup %s: %w", host, err)
	}

	want := dkimPublicPart(expectedPubTXT)
	if want == "" {
		return false, fmt.Errorf("expected public key is empty")
	}
	return slices.ContainsFunc(records, func(rec string) bool {
		return dkimPublicPart(rec) == want
	}), nil
}

// dkimPublicPart extracts and normalises the p= tag (the base64 public key) from
// a DKIM TXT record, stripping the whitespace resolvers and DNS UIs introduce.
func dkimPublicPart(txt string) string {
	// A chunked TXT record arrives already joined by net.Resolver; still strip
	// any interior whitespace the base64 may have picked up.
	for tag := range strings.SplitSeq(txt, ";") {
		tag = strings.TrimSpace(tag)
		if v, ok := strings.CutPrefix(tag, "p="); ok {
			return strings.Join(strings.Fields(v), "")
		}
	}
	return ""
}

func dnsIsNotFound(err error) bool {
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr) && dnsErr.IsNotFound
}
