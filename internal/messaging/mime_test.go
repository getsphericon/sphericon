package messaging_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/stretchr/testify/require"
)

// render serializes a built message to its raw MIME wire form for assertion.
func render(t *testing.T, msg messaging.EmailMessage) string {
	t.Helper()
	m, err := messaging.BuildMIME(msg)
	require.NoError(t, err)
	var buf bytes.Buffer
	_, err = m.WriteTo(&buf)
	require.NoError(t, err)
	return buf.String()
}

func TestBuildMIMEMultipartAlternative(t *testing.T) {
	raw := render(t, messaging.EmailMessage{
		From:     "sender@example.com",
		FromName: "Sender",
		To:       "rcpt@example.com",
		Subject:  "Hello",
		HTML:     "<p>hi</p>",
		Text:     "hi",
	})

	require.Contains(t, raw, "multipart/alternative")
	require.Contains(t, raw, "text/plain")
	require.Contains(t, raw, "text/html")
	require.Contains(t, raw, `From: "Sender" <sender@example.com>`)
	require.Contains(t, raw, "To: <rcpt@example.com>")
	require.Contains(t, raw, "Subject: Hello")

	// text/plain must come before text/html so clients render HTML as the richer part.
	require.Less(t, strings.Index(raw, "text/plain"), strings.Index(raw, "text/html"))
}

func TestBuildMIMEHTMLOnly(t *testing.T) {
	raw := render(t, messaging.EmailMessage{
		From: "sender@example.com", To: "rcpt@example.com", Subject: "s", HTML: "<p>hi</p>",
	})
	require.Contains(t, raw, "text/html")
	require.NotContains(t, raw, "multipart/alternative")
}

func TestBuildMIMETextOnly(t *testing.T) {
	raw := render(t, messaging.EmailMessage{
		From: "sender@example.com", To: "rcpt@example.com", Subject: "s", Text: "hi",
	})
	require.Contains(t, raw, "text/plain")
	require.NotContains(t, raw, "multipart/alternative")
}

func TestBuildMIMEInvalidFrom(t *testing.T) {
	_, err := messaging.BuildMIME(messaging.EmailMessage{From: "not-an-address", To: "rcpt@example.com"})
	require.Error(t, err)
}

// RFC 8058 one-click (ADR 0012): ListUnsubscribeURL emits both headers, the URI
// angle-bracketed and the One-Click POST directive.
func TestBuildMIMEListUnsubscribe(t *testing.T) {
	raw := render(t, messaging.EmailMessage{
		From: "sender@example.com", To: "rcpt@example.com", Subject: "s", HTML: "<p>hi</p>",
		ListUnsubscribeURL: "https://sphericon.test/e/u/tok",
	})
	require.Contains(t, raw, "List-Unsubscribe: <https://sphericon.test/e/u/tok>")
	require.Contains(t, raw, "List-Unsubscribe-Post: List-Unsubscribe=One-Click")
}

// Transactional/default sends carry no one-click header.
func TestBuildMIMENoListUnsubscribeByDefault(t *testing.T) {
	raw := render(t, messaging.EmailMessage{
		From: "sender@example.com", To: "rcpt@example.com", Subject: "s", Text: "hi",
	})
	require.NotContains(t, raw, "List-Unsubscribe")
}

func TestBuildMIMEReplyTo(t *testing.T) {
	msg := messaging.EmailMessage{From: "sender@example.com", To: "rcpt@example.com", Subject: "Hi", Text: "hi"}

	require.NotContains(t, render(t, msg), "Reply-To:", "no Reply-To unless one is set")

	msg.ReplyTo = "replies@example.com"
	require.Contains(t, render(t, msg), "Reply-To: <replies@example.com>")

	msg.ReplyTo = "not an address"
	_, err := messaging.BuildMIME(msg)
	require.ErrorContains(t, err, "reply-to")
}
