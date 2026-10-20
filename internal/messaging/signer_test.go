package messaging_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	emdkim "github.com/emersion/go-msgauth/dkim"
	"github.com/foxcpp/go-mockdns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/dnstest"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Fixture sending domains for workspace 1 (fixtures/sending_domains.yml):
//   id 1  mail.acme.com  verified,   selector "sphericon"
//   id 2  news.acme.com  unverified, selector "sphericon"

// dkimLookup serves pubTXT at mail.acme.com's DKIM name through a real resolver.
// Any other name answers NXDOMAIN, so a signature over the wrong domain or
// selector fails verification.
func dkimLookup(t *testing.T, pubTXT string) func(string) ([]string, error) {
	t.Helper()
	r := dnstest.Resolver(t, map[string]mockdns.Zone{
		"sphericon._domainkey.mail.acme.com.": {TXT: []string{pubTXT}},
	})
	return func(name string) ([]string, error) {
		return r.LookupTXT(context.Background(), name+".")
	}
}

func TestDKIMSignerVerifiedDomain(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	signer := messaging.NewDKIMSigner(env.DB.Scoped(fixtures.AcmeID), envCipher(t))

	dk, err := signer.DKIMSigner(ctx, "hello@mail.acme.com")
	require.NoError(t, err)
	assert.NotNil(t, dk, "a verified sending domain must yield a signer")
}

func TestDKIMSignerUnverifiedDomain(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	signer := messaging.NewDKIMSigner(env.DB.Scoped(fixtures.AcmeID), envCipher(t))

	dk, err := signer.DKIMSigner(ctx, "hello@news.acme.com")
	require.NoError(t, err)
	assert.Nil(t, dk, "an unverified sending domain must not be signed (slice 2)")
}

func TestDKIMSignerUnknownDomain(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	signer := messaging.NewDKIMSigner(env.DB.Scoped(fixtures.AcmeID), envCipher(t))

	dk, err := signer.DKIMSigner(ctx, "hello@nope.example")
	require.NoError(t, err)
	assert.Nil(t, dk, "a domain with no sending domain must not be signed")
}

// A verified domain in another workspace must not sign this workspace's mail.
func TestDKIMSignerWorkspaceScoped(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	signer := messaging.NewDKIMSigner(env.DB.Scoped(fixtures.GlobexID), envCipher(t))

	dk, err := signer.DKIMSigner(ctx, "hello@mail.acme.com")
	require.NoError(t, err)
	assert.Nil(t, dk, "mail.acme.com belongs to workspace 1, not 2")
}

func TestBuildSignedMIMESignsVerified(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	signer := messaging.NewDKIMSigner(env.DB.Scoped(fixtures.AcmeID), envCipher(t))

	m, err := messaging.BuildSignedMIME(ctx, messaging.EmailMessage{
		From:    "hello@mail.acme.com",
		To:      "rcpt@example.com",
		Subject: "signed",
		HTML:    "<p>hi</p>",
		Text:    "hi",
	}, signer)
	require.NoError(t, err)

	var buf bytes.Buffer
	_, err = m.WriteTo(&buf)
	require.NoError(t, err)

	raw := buf.String()
	assert.Contains(t, raw, "DKIM-Signature:")
	assert.Contains(t, raw, "d=mail.acme.com")
	assert.Contains(t, raw, "s=sphericon")
}

// The gate (slice 3): a workspace send from an unverified domain is rejected,
// not silently sent unsigned.
func TestBuildSignedMIMERejectsUnverified(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	signer := messaging.NewDKIMSigner(env.DB.Scoped(fixtures.AcmeID), envCipher(t))

	_, err := messaging.BuildSignedMIME(ctx, messaging.EmailMessage{
		From:    "hello@news.acme.com",
		To:      "rcpt@example.com",
		Subject: "unsigned",
		Text:    "hi",
	}, signer)
	assert.ErrorIs(t, err, messaging.ErrUnverifiedSendingDomain)
}

func TestBuildSignedMIMENilSigner(t *testing.T) {
	m, err := messaging.BuildSignedMIME(context.Background(), messaging.EmailMessage{
		From:    "hello@mail.acme.com",
		To:      "rcpt@example.com",
		Subject: "unsigned",
		Text:    "hi",
	}, nil)
	require.NoError(t, err)

	var buf bytes.Buffer
	_, err = m.WriteTo(&buf)
	require.NoError(t, err)
	assert.NotContains(t, buf.String(), "DKIM-Signature:")
}

// dkimHeaderTag unfolds the DKIM-Signature header from a raw message and returns
// its h= tag (the signed-header list).
func dkimHeaderTag(t *testing.T, raw string) string {
	t.Helper()
	var sig strings.Builder
	inSig := false
	for ln := range strings.SplitSeq(raw, "\r\n") {
		switch {
		case strings.HasPrefix(ln, "DKIM-Signature:"):
			inSig = true
			sig.WriteString(strings.TrimPrefix(ln, "DKIM-Signature:"))
		case inSig && (strings.HasPrefix(ln, " ") || strings.HasPrefix(ln, "\t")):
			sig.WriteString(ln)
		case inSig:
			inSig = false
		}
	}
	require.NotEmpty(t, sig.String(), "no DKIM-Signature header found")
	for tag := range strings.SplitSeq(sig.String(), ";") {
		if tag = strings.TrimSpace(tag); strings.HasPrefix(tag, "h=") {
			return strings.TrimPrefix(tag, "h=")
		}
	}
	t.Fatal("no h= tag in DKIM-Signature")
	return ""
}

// RFC 8058 §5 (ADR 0012): the one-click headers must be inside the DKIM-signed
// set (h=), or Gmail/Yahoo silently ignore one-click. Assert both are named in
// h= and that the signature still verifies with them present.
func TestDKIMSignsListUnsubscribeHeaders(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	signer := messaging.NewDKIMSigner(env.DB.Scoped(fixtures.AcmeID), envCipher(t))

	dom, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)
	pubTXT := dom.DkimPublicKey

	m, err := messaging.BuildSignedMIME(ctx, messaging.EmailMessage{
		From:               "hello@mail.acme.com",
		To:                 "rcpt@example.com",
		Subject:            "one-click",
		HTML:               "<p>hi</p>",
		Text:               "hi",
		ListUnsubscribeURL: "https://mail.acme.com/e/u/tok",
	}, signer)
	require.NoError(t, err)

	var buf bytes.Buffer
	_, err = m.WriteTo(&buf)
	require.NoError(t, err)
	raw := buf.String()

	h := strings.ToLower(dkimHeaderTag(t, raw))
	assert.Contains(t, h, "list-unsubscribe", "h= must cover List-Unsubscribe")
	assert.Contains(t, h, "list-unsubscribe-post", "h= must cover List-Unsubscribe-Post")
	// Simple header canonicalization (relaxed body) sidesteps go-mail's mid-token
	// fold of the long h= that breaks relaxed verification.
	assert.Contains(t, raw, "c=simple/relaxed")

	verifs, err := emdkim.VerifyWithOptions(bytes.NewReader(buf.Bytes()), &emdkim.VerifyOptions{
		LookupTXT: dkimLookup(t, pubTXT),
	})
	require.NoError(t, err)
	require.Len(t, verifs, 1)
	assert.NoError(t, verifs[0].Err, "signature must verify with the one-click headers present")
}

// A transactional send (no ListUnsubscribeURL) keeps the default relaxed signing
// path: it neither carries nor signs a List-Unsubscribe header, and verifies. The
// one-click headers and simple canonicalization are scoped to marketing sends, so
// transactional mail is untouched by ADR 0012.
func TestDKIMTransactionalUnchanged(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	signer := messaging.NewDKIMSigner(env.DB.Scoped(fixtures.AcmeID), envCipher(t))

	dom, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)
	pubTXT := dom.DkimPublicKey

	m, err := messaging.BuildSignedMIME(ctx, messaging.EmailMessage{
		From: "hello@mail.acme.com", To: "rcpt@example.com", Subject: "txn", Text: "hi",
	}, signer)
	require.NoError(t, err)

	var buf bytes.Buffer
	_, err = m.WriteTo(&buf)
	require.NoError(t, err)
	raw := buf.String()
	require.NotContains(t, raw, "List-Unsubscribe:")
	assert.NotContains(t, strings.ToLower(dkimHeaderTag(t, raw)), "list-unsubscribe",
		"transactional h= must not name the one-click headers")
	assert.Contains(t, raw, "c=relaxed/relaxed", "transactional stays on relaxed canonicalization")

	verifs, err := emdkim.VerifyWithOptions(bytes.NewReader(buf.Bytes()), &emdkim.VerifyOptions{
		LookupTXT: dkimLookup(t, pubTXT),
	})
	require.NoError(t, err)
	require.Len(t, verifs, 1)
	assert.NoError(t, verifs[0].Err)
}

// The signature over a multipart/alternative body must verify cryptographically
// against the domain's published public key — the go-mail body-hash must match
// the wire body. The fixture public key is injected via LookupTXT so the check
// needs no DNS.
func TestDKIMSignatureCryptoVerifies(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	signer := messaging.NewDKIMSigner(env.DB.Scoped(fixtures.AcmeID), envCipher(t))

	dom, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)
	pubTXT := dom.DkimPublicKey

	m, err := messaging.BuildSignedMIME(ctx, messaging.EmailMessage{
		From:    "hello@mail.acme.com",
		To:      "rcpt@example.com",
		Subject: "multipart signed",
		HTML:    "<p>hello <strong>world</strong></p>",
		Text:    "hello world",
	}, signer)
	require.NoError(t, err)

	var buf bytes.Buffer
	_, err = m.WriteTo(&buf)
	require.NoError(t, err)

	verifs, err := emdkim.VerifyWithOptions(bytes.NewReader(buf.Bytes()), &emdkim.VerifyOptions{
		LookupTXT: dkimLookup(t, pubTXT),
	})
	require.NoError(t, err)
	require.Len(t, verifs, 1)
	require.NoError(t, verifs[0].Err, "DKIM signature must verify against the published key")
	assert.Equal(t, "mail.acme.com", verifs[0].Domain)
}
