package messaging_test

import (
	"context"
	"strings"
	"testing"

	emdkim "github.com/emersion/go-msgauth/dkim"
	"github.com/foxcpp/go-mockdns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/dnstest"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
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

func TestDKIMKeysVerifiedDomain(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	keys := messaging.NewDKIMKeys(env.DB.Scoped(fixtures.AcmeID), env.Cipher)

	dk, err := keys.Key(ctx, "hello@mail.acme.com")
	require.NoError(t, err)
	assert.NotNil(t, dk, "a verified sending domain must yield a key")
}

func TestDKIMKeysUnverifiedDomain(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	keys := messaging.NewDKIMKeys(env.DB.Scoped(fixtures.AcmeID), env.Cipher)

	dk, err := keys.Key(ctx, "hello@news.acme.com")
	require.NoError(t, err)
	assert.Nil(t, dk, "an unverified sending domain must not be signed (slice 2)")
}

func TestDKIMKeysLostVerification(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	keys := messaging.NewDKIMKeys(env.DB.Scoped(fixtures.AcmeID), env.Cipher)

	dk, err := keys.Key(ctx, "hello@"+fixtures.SendingDomainLostDomain)
	require.NoError(t, err)
	assert.Nil(t, dk, "a domain whose verification was lost must not be signed")
	has, err := messaging.HasVerifiedSendingDomain(ctx, env.DB.Scoped(fixtures.AcmeID), "hello@"+fixtures.SendingDomainLostDomain)
	require.NoError(t, err)
	assert.False(t, has)
}

func TestDKIMKeysUnknownDomain(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	keys := messaging.NewDKIMKeys(env.DB.Scoped(fixtures.AcmeID), env.Cipher)

	dk, err := keys.Key(ctx, "hello@nope.example")
	require.NoError(t, err)
	assert.Nil(t, dk, "a domain with no sending domain must not be signed")
}

// A verified domain in another workspace must not sign this workspace's mail.
func TestDKIMKeysWorkspaceScoped(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	keys := messaging.NewDKIMKeys(env.DB.Scoped(fixtures.GlobexID), env.Cipher)

	dk, err := keys.Key(ctx, "hello@mail.acme.com")
	require.NoError(t, err)
	assert.Nil(t, dk, "mail.acme.com belongs to workspace 1, not 2")
}

func TestBuildSignedMessageSignsVerified(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	keys := messaging.NewDKIMKeys(env.DB.Scoped(fixtures.AcmeID), env.Cipher)

	wire, err := messaging.BuildSignedMessage(ctx, messaging.EmailMessage{
		From:    "hello@mail.acme.com",
		To:      "rcpt@example.com",
		Subject: "signed",
		HTML:    "<p>hi</p>",
		Text:    "hi",
	}, keys)
	require.NoError(t, err)
	raw := string(wire.Bytes)

	headsWithSignature := strings.HasPrefix(raw, "DKIM-Signature:")
	messageIDHeader := "Message-ID: " + wire.MessageID
	assert.True(t, headsWithSignature, "the signature heads the message")
	assert.Contains(t, raw, "d=mail.acme.com")
	assert.Contains(t, raw, "s=sphericon")
	assert.Contains(t, raw, messageIDHeader)
	assert.Equal(t, "<hello@mail.acme.com>", wire.Sender)
	assert.Equal(t, []string{"<rcpt@example.com>"}, wire.Recipients)
}

// The gate (slice 3): a workspace send from an unverified domain is rejected,
// not silently sent unsigned.
func TestBuildSignedMessageRejectsUnverified(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	keys := messaging.NewDKIMKeys(env.DB.Scoped(fixtures.AcmeID), env.Cipher)

	_, err := messaging.BuildSignedMessage(ctx, messaging.EmailMessage{
		From:    "hello@news.acme.com",
		To:      "rcpt@example.com",
		Subject: "unsigned",
		Text:    "hi",
	}, keys)
	assert.ErrorIs(t, err, messaging.ErrUnverifiedSendingDomain)
}

// System mail has no DKIM keys and goes out unsigned.
func TestBuildSignedMessageWithoutKeysIsUnsigned(t *testing.T) {
	wire, err := messaging.BuildSignedMessage(context.Background(), messaging.EmailMessage{
		From:    "hello@mail.acme.com",
		To:      "rcpt@example.com",
		Subject: "unsigned",
		Text:    "hi",
	}, nil)
	require.NoError(t, err)
	raw := string(wire.Bytes)
	messageIDHeader := "Message-ID: " + wire.MessageID
	assert.NotContains(t, raw, "DKIM-Signature:")
	assert.Contains(t, raw, messageIDHeader)
}

// dkimSignature unfolds the DKIM-Signature header of a raw message and returns
// its tags by name.
func dkimSignature(t *testing.T, raw string) map[string]string {
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
	tags := map[string]string{}
	for tag := range strings.SplitSeq(sig.String(), ";") {
		name, value, _ := strings.Cut(strings.TrimSpace(tag), "=")
		tags[name] = strings.Join(strings.Fields(value), "")
	}
	return tags
}

// refold rewrites the header block the way an intermediate MTA may: every header is
// unfolded and then folded again at each of its spaces, with a tab as the folding
// whitespace. The body is left as it is.
func refold(raw string) string {
	head, body, _ := strings.Cut(raw, "\r\n\r\n")
	unfolded := strings.NewReplacer("\r\n ", " ", "\r\n\t", " ").Replace(head)
	return strings.ReplaceAll(unfolded, " ", "\r\n\t") + "\r\n\r\n" + body
}

// verifyDKIM checks the one signature of raw against mail.acme.com's published key.
func verifyDKIM(t *testing.T, raw, pubTXT string) {
	t.Helper()
	verifs, err := emdkim.VerifyWithOptions(strings.NewReader(raw), &emdkim.VerifyOptions{
		LookupTXT: dkimLookup(t, pubTXT),
	})
	require.NoError(t, err)
	require.Len(t, verifs, 1)
	require.NoError(t, verifs[0].Err, "the DKIM signature must verify against the published key")
	assert.Equal(t, "mail.acme.com", verifs[0].Domain)
}

// RFC 8058 §5 (ADR 0012): the one-click headers must be inside the DKIM-signed
// set (h=), or Gmail/Yahoo silently ignore one-click. Marketing mail is signed with
// relaxed header canonicalization, so the signature still verifies after an
// intermediate MTA refolds its headers.
func TestDKIMSignsListUnsubscribeHeadersAndSurvivesRefolding(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	keys := messaging.NewDKIMKeys(env.DB.Scoped(fixtures.AcmeID), env.Cipher)

	dom, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)

	wire, err := messaging.BuildSignedMessage(ctx, messaging.EmailMessage{
		From:               "hello@mail.acme.com",
		FromName:           "Acme News",
		To:                 "rcpt@example.com",
		Subject:            "one-click unsubscribe from the weekly digest",
		HTML:               "<p>hi</p>",
		Text:               "hi",
		ListUnsubscribeURL: "https://mail.acme.com/e/u/tok",
	}, keys)
	require.NoError(t, err)
	raw := string(wire.Bytes)

	sig := dkimSignature(t, raw)
	h := strings.Split(strings.ToLower(sig["h"]), ":")
	assert.Contains(t, h, "list-unsubscribe", "h= must cover List-Unsubscribe")
	assert.Contains(t, h, "list-unsubscribe-post", "h= must cover List-Unsubscribe-Post")
	assert.Equal(t, "relaxed/relaxed", sig["c"])
	verifyDKIM(t, raw, dom.DkimPublicKey)

	refolded := refold(raw)
	require.NotEqual(t, raw, refolded)
	require.Contains(t, refolded, "List-Unsubscribe:\r\n\t<https://mail.acme.com/e/u/tok>")
	verifyDKIM(t, refolded, dom.DkimPublicKey)
}

// A transactional send (no ListUnsubscribeURL) neither carries nor signs a
// List-Unsubscribe header, and verifies: the one-click headers are scoped to
// marketing sends, so transactional mail is untouched by ADR 0012.
func TestDKIMTransactionalLeavesOneClickOut(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	keys := messaging.NewDKIMKeys(env.DB.Scoped(fixtures.AcmeID), env.Cipher)

	dom, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)

	wire, err := messaging.BuildSignedMessage(ctx, messaging.EmailMessage{
		From: "hello@mail.acme.com", To: "rcpt@example.com", Subject: "txn", Text: "hi",
	}, keys)
	require.NoError(t, err)
	raw := string(wire.Bytes)
	require.NotContains(t, raw, "List-Unsubscribe:")
	sig := dkimSignature(t, raw)
	signed := strings.ToLower(sig["h"])
	assert.NotContains(t, signed, "list-unsubscribe",
		"transactional h= must not name the one-click headers")
	assert.Equal(t, "relaxed/relaxed", sig["c"])
	verifyDKIM(t, raw, dom.DkimPublicKey)
}

// The signature over a multipart/alternative body must verify cryptographically
// against the domain's published public key: the body hash must match the wire
// body. The fixture public key is injected via LookupTXT so the check needs no DNS.
func TestDKIMSignatureCryptoVerifies(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	keys := messaging.NewDKIMKeys(env.DB.Scoped(fixtures.AcmeID), env.Cipher)

	dom, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)

	wire, err := messaging.BuildSignedMessage(ctx, messaging.EmailMessage{
		From:    "hello@mail.acme.com",
		To:      "rcpt@example.com",
		Subject: "multipart signed",
		HTML:    "<p>hello <strong>world</strong></p>",
		Text:    "hello world",
	}, keys)
	require.NoError(t, err)
	raw := string(wire.Bytes)
	verifyDKIM(t, raw, dom.DkimPublicKey)
}
