//go:build e2e

package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func messageWith(headers map[string][]string) Message {
	return Message{Headers: headers}
}

func TestMessageUnsubscribeURLPrefersHTTPOverMailto(t *testing.T) {
	t.Parallel()
	m := messageWith(map[string][]string{
		"List-Unsubscribe": {"<mailto:unsub@example.test?subject=u>, <https://app.example.test/u/abc>"},
	})
	assert.Equal(t, "https://app.example.test/u/abc", m.UnsubscribeURL())
}

func TestMessageUnsubscribeURLAcceptsPlainHTTP(t *testing.T) {
	t.Parallel()
	m := messageWith(map[string][]string{"List-Unsubscribe": {"<http://localhost/u/1>"}})
	assert.Equal(t, "http://localhost/u/1", m.UnsubscribeURL())
}

func TestMessageUnsubscribeURLIsEmptyWithoutHTTPTarget(t *testing.T) {
	t.Parallel()
	assert.Empty(t, messageWith(nil).UnsubscribeURL())
	assert.Empty(t, messageWith(map[string][]string{"List-Unsubscribe": {"<mailto:unsub@example.test>"}}).UnsubscribeURL())
}

func TestMessageOneClickBody(t *testing.T) {
	t.Parallel()
	m := messageWith(map[string][]string{"List-Unsubscribe-Post": {"List-Unsubscribe=One-Click"}})
	assert.Equal(t, "List-Unsubscribe=One-Click", m.OneClickBody())
	assert.Empty(t, messageWith(nil).OneClickBody())
}

// A link is read the way a mail client reads it, whatever spelling of the markup the app writes.
func TestMessageLinkReadsTheHTMLAsAClientDoes(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"double quotes":      `<a href="https://app.test/e/c/tok?a=1&amp;b=2">x</a>`,
		"single quotes":      `<a href='https://app.test/e/c/tok?a=1&amp;b=2'>x</a>`,
		"no quotes":          `<a href=https://app.test/e/c/tok?a=1&amp;b=2>x</a>`,
		"upper-case name":    `<A HREF="https://app.test/e/c/tok?a=1&amp;b=2">x</A>`,
		"after another link": `<a href="https://example.com/e/x">y</a><a href="https://app.test/e/c/tok?a=1&b=2">x</a>`,
		"apostrophe nearby":  `<a href="https://app.test/e/c/tok?a=1&amp;b=2" title="it's">x</a>`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, "https://app.test/e/c/tok?a=1&b=2", Message{HTML: body}.Link("/e/c/"))
		})
	}
}

func TestMessageLinkIsEmptyWithoutAMatchingLink(t *testing.T) {
	t.Parallel()
	assert.Empty(t, Message{HTML: `<a href="mailto:x@example.test/e/c/">x</a><p>https://app.test/e/c/tok</p>`}.Link("/e/c/"))
	assert.Empty(t, Message{}.Link("/e/c/"))
}

func TestMessageImageReadsTheSrc(t *testing.T) {
	t.Parallel()
	m := Message{HTML: `<img src='https://app.test/logo.png'><IMG alt="" SRC=https://app.test/e/o/tok>`}
	assert.Equal(t, "https://app.test/e/o/tok", m.Image("/e/o/"))
	assert.Empty(t, m.Link("/e/o/"), "an <img> is not a link")
}
