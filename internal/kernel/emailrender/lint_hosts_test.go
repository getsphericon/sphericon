package emailrender_test

import (
	"html"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/kernel/emailrender"
)

func reservedHosts(report emailrender.Report) []emailrender.Finding {
	return lo.Filter(report.Warnings, func(f emailrender.Finding, _ int) bool {
		return f.Code == emailrender.CodeReservedHost
	})
}

// A link or an image at a name RFC 2606 and RFC 6761 reserve for examples and tests is a
// placeholder left in the message (ADR 0188).
func TestLintWarnsOnALinkOrImageAtAReservedHost(t *testing.T) {
	for name, tc := range map[string]struct{ body, element, detail string }{
		"a link to example.com": {
			`<a href="https://example.com/x">Shop</a>`, `<a href="https://example.com/x">`, "https://example.com/x",
		},
		"an image on a subdomain of example.org": {
			`<img src="https://cdn.example.org/a.png" alt="">`, `<img src="https://cdn.example.org/a.png">`, "https://cdn.example.org/a.png",
		},
		"a subdomain of example.org": {
			`<a href="https://news.example.org">News</a>`, `<a href="https://news.example.org">`, "https://news.example.org",
		},
		"example.net": {
			`<a href="http://example.net">Go</a>`, `<a href="http://example.net">`, "http://example.net",
		},
		"the bare example name": {
			`<a href="https://example/">Go</a>`, `<a href="https://example/">`, "https://example/",
		},
		"a host ending in .test": {
			`<a href="https://shop.acme.test/cart">Go</a>`, `<a href="https://shop.acme.test/cart">`, "https://shop.acme.test/cart",
		},
		"a host ending in .invalid": {
			`<a href="https://acme.invalid">Go</a>`, `<a href="https://acme.invalid">`, "https://acme.invalid",
		},
		"a host ending in .localhost": {
			`<img src="https://app.localhost:8443/a.png" alt="">`, `<img src="https://app.localhost:8443/a.png">`, "https://app.localhost:8443/a.png",
		},
		"a host in upper case": {
			`<a href="https://WWW.Example.COM/">Go</a>`, `<a href="https://WWW.Example.COM/">`, "https://WWW.Example.COM/",
		},
		"a URL with credentials": {
			`<a href="https://user:pass@example.com/">Go</a>`, `<a href="https://user:pass@example.com/">`, "https://user:pass@example.com/",
		},
	} {
		t.Run(name, func(t *testing.T) {
			report := emailrender.Lint("Hi", tc.body, nil)
			found := reservedHosts(report)

			require.NoError(t, report.Error)
			require.Len(t, found, 1)
			assert.Equal(t, tc.element, found[0].Element)
			assert.Equal(t, tc.detail, found[0].Detail)
			assert.NotEmpty(t, found[0].Message)
		})
	}
}

// A host that comes from a merge tag is decided per recipient (ADR 0188): the lint renders with no
// recipient data, so the tag leaves an empty or a partial host, which is not checked.
func TestLintLeavesAHostAloneWhenItIsNotReserved(t *testing.T) {
	for name, body := range map[string]string{
		"a name that only starts like one":  `<a href="https://examples.com">Go</a>`,
		"a name that only ends like one":    `<a href="https://notexample.com">Go</a>`,
		"a reserved name inside a label":    `<a href="https://bitly.example-shop.org">Go</a>`,
		"a reserved name in the path":       `<a href="https://acme.co/example.com">Go</a>`,
		"a merge tag as the whole URL":      `<a href="{{ url }}">Go</a>`,
		"a merge tag as the host":           `<a href="https://{{ host }}/x">Go</a>`,
		"a merge tag before a reserved one": `<a href="https://{{ sub }}.example.com">Go</a>`,
		"a merge tag as the top level":      `<a href="https://example.{{ tld }}">Go</a>`,
		"a merge tag inside the host":       `<img src="https://cdn.{{ region }}.example.org/a.png" alt="">`,
		"a relative link":                   `<a href="/x">Go</a>`,
		"an anchor":                         `<a href="#top">Go</a>`,
		"a mail address":                    `<a href="mailto:hello@example.com">Mail</a>`,
		"a telephone number":                `<a href="tel:+15550100">Call</a>`,
		"a URL that does not parse":         `<a href="https://exa mple.com">Go</a>`,
	} {
		t.Run(name, func(t *testing.T) {
			report := emailrender.Lint("Hi", body, nil)
			found := reservedHosts(report)

			require.NoError(t, report.Error)
			assert.Empty(t, found)
		})
	}
}

// A reserved name warns under any subdomains.
func TestLintWarnsOnAReservedHostWhateverItsSubdomains(t *testing.T) {
	label := rapid.StringMatching(`[a-z0-9]([a-z0-9-]{0,10}[a-z0-9])?`)
	reserved := []string{"example", "example.com", "example.net", "example.org", "test", "invalid", "localhost"}
	rapid.Check(t, func(t *rapid.T) {
		labels := rapid.SliceOfN(label, 1, 4).Draw(t, "subdomains")
		base := rapid.SampledFrom(reserved).Draw(t, "reserved")
		host := strings.Join(append(labels, base), ".")

		report := emailrender.Lint("Hi", `<a href="https://`+host+`/p">Go</a>`, nil)
		found := reservedHosts(report)

		assert.Len(t, found, 1, host)
	})
}

func TestLintNamesEachReservedLinkOnce(t *testing.T) {
	report := emailrender.Lint("Hi",
		`<a href="https://example.com">One</a><a href="https://example.com">Two</a><a href="https://example.net">Three</a>`, nil)
	details := lo.Map(reservedHosts(report), func(f emailrender.Finding, _ int) string { return f.Detail })

	assert.Equal(t, []string{"https://example.com", "https://example.net"}, details)
}

// Any address a person writes into a link is read without a panic, and one link warns at most once.
func FuzzLintReadsAnyLinkAddress(f *testing.F) {
	for _, seed := range []string{
		"https://example.com/x", "https://{{ host }}/x", "{{ url }}", "mailto:a@example.com",
		"//cdn.example.org/a.png", "https://[::1]:8080/", "https://.example.com", "http://exa mple.com", "%zz",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, href string) {
		report := emailrender.Lint("Hi", `<a href="`+html.EscapeString(href)+`">Go</a>`, nil)
		found := reservedHosts(report)

		assert.LessOrEqual(t, len(found), 1)
	})
}
