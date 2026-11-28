package emailrender

import (
	"net/url"
	"slices"
	"strings"

	"golang.org/x/net/html"
)

// urlKind is what the lint can read of a link's or an image's URL.
type urlKind int

const (
	// urlNoHost is a URL with no host the author wrote: empty, relative, an anchor, one that does
	// not parse, or one whose host comes from a merge tag. The lint renders with no recipient data,
	// so a merge tag renders empty: `{{ url }}` leaves "", `https://{{ host }}/x` leaves
	// `https:///x`, and a tag inside a host leaves an empty label (`https://{{ sub }}.example.com`
	// leaves `https://.example.com`). Such a host is decided per recipient and is not checked.
	// After the render an empty or a relative URL the author wrote looks the same as what a merge
	// tag left, so neither is checked.
	urlNoHost urlKind = iota
	// urlWeb is a URL with a host the author wrote in full, such as `https://example.com/x` or
	// `//cdn.example.org/a.png`.
	urlWeb
	// urlOpaque is a URL of a scheme with no host: `mailto:`, `tel:`, `data:`, `cid:`.
	urlOpaque
)

// lintURL is a link's or an image's URL as classifyURL reads it.
type lintURL struct {
	Kind urlKind
	// Scheme is lower-case, "" for a relative URL; it is set for every kind, so a check of the
	// scheme reads it alongside Kind (a merge-tag host is urlNoHost even on `http`).
	Scheme string
	// Host is the lower-case host name, without a port or credentials; set only for urlWeb.
	Host string
}

// classifyURL reads a rendered href or src for the checks of its host and scheme, telling a host
// the author wrote from one that comes from a merge tag (see urlNoHost).
func classifyURL(raw string) lintURL {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return lintURL{Kind: urlNoHost}
	}
	scheme := strings.ToLower(u.Scheme)
	if u.Opaque != "" {
		return lintURL{Kind: urlOpaque, Scheme: scheme}
	}
	host := strings.ToLower(u.Hostname())
	if host == "" || slices.Contains(strings.Split(host, "."), "") {
		return lintURL{Kind: urlNoHost, Scheme: scheme}
	}
	return lintURL{Kind: urlWeb, Scheme: scheme, Host: host}
}

// urlOf is the URL an element points at: a link's href or an image's src; ok is false for any
// other element and for one without the attribute.
func urlOf(n *html.Node) (string, bool) {
	switch n.Data {
	case "a":
		return attr(n, "href"), hasAttr(n, "href")
	case "img":
		return attr(n, "src"), hasAttr(n, "src")
	}
	return "", false
}

// urlChecks are the checks of a link's or an image's URL.
func (l *linter) urlChecks(n *html.Node, el string) {
	raw, ok := urlOf(n)
	if !ok {
		return
	}
	u := classifyURL(raw)
	if u.Kind == urlWeb && reservedHost(u.Host) {
		l.add(Finding{
			Code: CodeReservedHost, Message: "this address is a name reserved for examples and tests, not a real site",
			Element: el, Detail: clip(raw, maxURLDetail),
		})
	}
}

// maxURLDetail caps the URL a finding quotes.
const maxURLDetail = 200

// The names RFC 2606 and RFC 6761 reserve for examples and tests (ADR 0188): each name and its
// subdomains, and every name under each top-level domain.
var (
	reservedNames = []string{"example", "example.com", "example.net", "example.org"}
	reservedTLDs  = []string{"test", "invalid", "localhost"}
)

// reservedHost reports whether a lower-case host name is one of reservedNames or under one, or
// ends in one of reservedTLDs. A name is matched on whole labels: `examples.com` is not reserved.
func reservedHost(host string) bool {
	under := func(name string) bool { return strings.HasSuffix(host, "."+name) }
	return slices.Contains(reservedNames, host) || slices.ContainsFunc(reservedNames, under) ||
		slices.ContainsFunc(reservedTLDs, under)
}
