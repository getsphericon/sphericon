//go:build e2e

package e2e

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// angleURL picks the <...> entries of a List-Unsubscribe value (RFC 2369).
var angleURL = regexp.MustCompile(`<([^>]*)>`)

// Message is a delivered email with the fields the scenarios assert on.
type Message struct {
	ID      string
	From    Address
	To      []Address
	Subject string
	Text    string
	HTML    string
	// Headers are the raw header values keyed by canonical name, so a scenario can
	// read List-Unsubscribe and List-Unsubscribe-Post exactly as they went on the wire.
	Headers map[string][]string
}

// Header returns the first value of the named header ("" when absent).
func (m Message) Header(name string) string {
	return first(m.Headers[http.CanonicalHeaderKey(name)])
}

func first(v []string) string {
	if len(v) == 0 {
		return ""
	}
	return v[0]
}

// UnsubscribeURL is the http(s) target in List-Unsubscribe (preferred over mailto:),
// or "" when the header is absent or carries none.
func (m Message) UnsubscribeURL() string {
	for _, e := range angleURL.FindAllStringSubmatch(m.Header("List-Unsubscribe"), -1) {
		if strings.HasPrefix(e[1], "https://") || strings.HasPrefix(e[1], "http://") {
			return e[1]
		}
	}
	return ""
}

// OneClickBody is the List-Unsubscribe-Post value (RFC 8058), the form body of the
// one-click request; "" when the header is absent.
func (m Message) OneClickBody() string {
	return m.Header("List-Unsubscribe-Post")
}

// Link is the first http(s) URL an <a href> of the email's HTML points at whose path holds
// pathPart, or "" when there is none.
//
// Both read the HTML the way a mail client does, through an HTML parser: an attribute may be
// quoted with either quote or not at all, its name may be in any case, and a character reference
// in it (&amp;) is decoded. A pattern over the markup sees one spelling of each, so a link the
// app writes in another one reads as missing, or as a URL no client would request.
func (m Message) Link(pathPart string) string { return m.urlIn("a", "href", pathPart) }

// Image is Link for an <img src>.
func (m Message) Image(pathPart string) string { return m.urlIn("img", "src", pathPart) }

func (m Message) urlIn(tag, attr, pathPart string) string {
	doc, err := html.Parse(strings.NewReader(m.HTML))
	if err != nil {
		return ""
	}
	for n := range doc.Descendants() {
		if n.Type != html.ElementNode || n.Data != tag {
			continue
		}
		for _, a := range n.Attr {
			if a.Key != attr {
				continue
			}
			u, err := url.Parse(strings.TrimSpace(a.Val))
			if err == nil && (u.Scheme == "https" || u.Scheme == "http") && strings.Contains(u.Path, pathPart) {
				return u.String()
			}
		}
	}
	return ""
}
