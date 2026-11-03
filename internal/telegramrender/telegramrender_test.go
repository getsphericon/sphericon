package telegramrender_test

import (
	"net/netip"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/telegramrender"
)

func TestRenderTable(t *testing.T) {
	bindings := map[string]any{"first_name": "Ann <b>x</b>", "city": "Berlin & Co"}
	cases := map[string]struct{ text, want string }{
		"plain text":                   {"Hello", "Hello"},
		"merge tag values are escaped": {"Hi {{ first_name }}!", "Hi Ann &lt;b&gt;x&lt;/b&gt;!"},
		"ampersand in a value":         {"{{ city }}", "Berlin &amp; Co"},
		"allowed tags stay":            {"<b>a</b> <i>b</i> <u>c</u> <s>d</s> <code>e</code> <tg-spoiler>f</tg-spoiler>", "<b>a</b> <i>b</i> <u>c</u> <s>d</s> <code>e</code> <tg-spoiler>f</tg-spoiler>"},
		"other tags lose the tag":      {"<p>one</p><div>two</div><span>three</span>", "onetwothree"},
		"script is dropped":            {"a<script>alert(1)</script>b", "ab"},
		"unclosed tag is closed":       {"<b>bold <i>both", "<b>bold <i>both</i></b>"},
		"misnested tags are repaired":  {"<b><i>x</b>y</i>", "<b><i>x</i></b><i>y</i>"},
		"stray end tag is dropped":     {"x</b>y", "xy"},
		"link with https":              {`<a href="https://example.com/a?x=1&y=2">go</a>`, `<a href="https://example.com/a?x=1&amp;y=2">go</a>`},
		"link with javascript":         {`<a href="javascript:alert(1)">go</a>`, "go"},
		"link without href":            {`<a name="x">go</a>`, "go"},
		"relative link":                {`<a href="/path">go</a>`, "go"},
		"other attributes dropped":     {`<b onclick="x()" style="color:red">b</b>`, "<b>b</b>"},
		"line break becomes newline":   {"a<br>b", "a\nb"},
		"nothing inside code":          {"<code>a <b>b</b></code>", "<code>a b</code>"},
		"pre keeps its text":           {"<pre>x < y</pre>", "<pre>x &lt; y</pre>"},
		"quote":                        {"<blockquote>q</blockquote>", "<blockquote>q</blockquote>"},
		"edges are trimmed":            {"  hi  ", "hi"},
		"a conditional":                {"{% if city %}in {{ city }}{% endif %}", "in Berlin &amp; Co"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := telegramrender.Render(c.text, bindings)

			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}

func TestRenderRefusesATemplateThatDoesNotRender(t *testing.T) {
	_, err := telegramrender.Render("{% if %}", nil)

	require.Error(t, err)
	require.Error(t, telegramrender.Check("{{ x | nofilter }}"))
	require.NoError(t, telegramrender.Check("Hi {{ first_name }}"))
}

func TestLengths(t *testing.T) {
	assert.Equal(t, 5, telegramrender.VisibleLength("<b>a&lt;b</b>cd"), "tags are no text and an entity is one character")
	assert.Equal(t, 2, telegramrender.WrittenLength("😀"), "UTF-16 code units, as the editor counts")
	assert.Equal(t, 4, telegramrender.WrittenLength("<b>x"))
}

var allowedTags = map[string]bool{
	"b": true, "strong": true, "i": true, "em": true, "u": true, "ins": true, "s": true,
	"strike": true, "del": true, "a": true, "code": true, "pre": true, "tg-spoiler": true, "blockquote": true,
}

// The output of Render, for any text, is HTML that parses to the allowed subset only, with an
// http(s) href as the one attribute, and rendering it again changes nothing.
func TestRenderAlwaysYieldsTheAllowedSubset(t *testing.T) {
	pieces := rapid.SampledFrom([]string{
		"<b>", "</b>", "<i>", "</i>", "<a href=\"https://e.com\">", "<a href=\"javascript:x\">", "</a>", "<code>", "</code>",
		"<pre>", "</pre>", "<script>", "</script>", "<p>", "<img src=x onerror=y>", "<tg-spoiler>", "<", ">", "&", "&amp;", "&lt;",
		"\"", "text", " ", "\n", "{{ name }}", "<!--", "-->", "<br/>", "<blockquote>", "<u onclick=z>",
	})
	rapid.Check(t, func(t *rapid.T) {
		text := strings.Join(rapid.SliceOf(pieces).Draw(t, "text"), "")

		out, err := telegramrender.Render(text, map[string]any{"name": "<b>N&"})
		if err != nil {
			t.Skip()
		}

		z := html.NewTokenizer(strings.NewReader(out))
		var open []string
		for tt := z.Next(); tt != html.ErrorToken; tt = z.Next() {
			switch tt {
			case html.StartTagToken:
				name, hasAttr := z.TagName()
				require.Truef(t, allowedTags[string(name)], "tag %s in %q", name, out)
				if hasAttr {
					key, val, _ := z.TagAttr()
					require.Equal(t, "a", string(name))
					require.Equal(t, "href", string(key))
					require.Regexp(t, `^https?://`, string(val))
				}
				open = append(open, string(name))
			case html.EndTagToken:
				name, _ := z.TagName()
				require.NotEmpty(t, open, out)
				require.Equal(t, open[len(open)-1], string(name), out)
				open = open[:len(open)-1]
			case html.SelfClosingTagToken, html.CommentToken, html.DoctypeToken:
				t.Fatalf("unexpected token in %q", out)
			case html.TextToken, html.ErrorToken:
			}
		}
		require.Empty(t, open, out)
		again, err := telegramrender.Render(out, nil)
		require.NoError(t, err)
		require.Equal(t, out, again)
	})
}

func TestLimitIsTheCaptionLimitWithAnImage(t *testing.T) {
	assert.Equal(t, 1024, telegramrender.Limit(true))
	assert.Equal(t, 4096, telegramrender.Limit(false))
}

func TestCheckImageTakesOnlyAbsoluteHTTPS(t *testing.T) {
	for url, ok := range map[string]bool{
		"": true, "https://cdn.example.com/a.png": true,
		"http://cdn.example.com/a.png": false, "/a.png": false, "javascript:x": false,
	} {
		err := telegramrender.CheckImage(url)
		assert.Equal(t, ok, err == nil, url)
	}
}

// refusedImageURLs are the forms of an internal target the Bot API server must not be sent to.
var refusedImageURLs = []string{
	"https://user@cdn.example.com/a.png", "https://user:pw@cdn.example.com/a.png", "https://@cdn.example.com/a.png",
	"https://localhost/a.png", "https://LOCALHOST:8443/a.png", "https://localhost./a.png", "https://app.localhost/a.png",
	"https://printer.local/a.png", "https://db.internal/a.png", "https://intranet/a.png", "https://metadata/a.png",
	"https://127.0.0.1/a.png", "https://127.1.2.3:9000/a.png", "https://10.0.0.5/a.png", "https://172.16.0.1/a.png",
	"https://172.31.255.255/a.png", "https://192.168.1.1/a.png", "https://169.254.169.254/latest/meta-data",
	"https://0.0.0.0/a.png", "https://100.64.0.1/a.png", "https://100.127.255.254/a.png", "https://0.1.2.3/a.png",
	"https://[::1]/a.png", "https://[::]/a.png", "https://[fe80::1]/a.png", "https://[fc00::1]/a.png",
	"https://[fd12:3456::1]/a.png", "https://[::ffff:127.0.0.1]/a.png", "https://[::ffff:10.0.0.1]/a.png",
	"https://[::ffff:169.254.169.254]/a.png", "https://[64:ff9b::7f00:1]/a.png", "https://[fe80::1%25eth0]/a.png",
	"https://2130706433/a.png", "https://0x7f000001/a.png", "https://0x7f.1/a.png", "https://127.1/a.png",
}

func TestCheckImageRefusesInternalTargets(t *testing.T) {
	for _, u := range refusedImageURLs {
		require.ErrorIs(t, telegramrender.CheckImage(u), telegramrender.ErrContent, u)
	}
}

func TestCheckImageTakesPublicHosts(t *testing.T) {
	for _, u := range []string{
		"https://cdn.example.com/a.png", "https://example.com:8443/a.png?x=1", "https://8.8.8.8/a.png",
		"https://[2606:4700:4700::1111]/a.png", "https://100.63.255.255/a.png", "https://172.32.0.1/a.png",
		"https://img.localhost.example.com/a.png", "https://notlocal.com/a.png", "https://CDN.Example.COM./a.png",
	} {
		require.NoError(t, telegramrender.CheckImage(u), u)
	}
}

func TestCheckImageErrorsAreReadable(t *testing.T) {
	require.ErrorContains(t, telegramrender.CheckImage("https://localhost/a.png"), "public host")
	require.ErrorContains(t, telegramrender.CheckImage("https://u:p@example.com/a.png"), "user name")
}

func TestCheckImageRefusesEveryInternalAddressSpelling(t *testing.T) {
	blocked := []string{
		"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16", "0.0.0.0/8",
		"100.64.0.0/10", "224.0.0.0/4", "240.0.0.0/4", "::1/128", "fe80::/10", "fc00::/7", "ff00::/8",
	}
	rapid.Check(t, func(t *rapid.T) {
		p := netip.MustParsePrefix(rapid.SampledFrom(blocked).Draw(t, "range"))
		noise := rapid.SliceOfN(rapid.Byte(), 16, 16).Draw(t, "noise")
		// Keep the prefix's bits and take the rest from the noise.
		base := p.Addr().As16()
		bits := p.Bits()
		if p.Addr().Is4() {
			bits += 96
		}
		for i := range base {
			keep := min(max(bits-8*i, 0), 8)
			mask := ^byte(0) << (8 - keep)
			base[i] = base[i]&mask | noise[i]&^mask
		}
		addr := netip.AddrFrom16(base)
		hosts := []string{"[" + addr.String() + "]"}
		if p.Addr().Is4() {
			hosts = []string{addr.Unmap().String(), "[" + addr.String() + "]"}
		}
		host := rapid.SampledFrom(hosts).Draw(t, "form")
		require.Error(t, telegramrender.CheckImage("https://"+host+"/a.png"), host)
	})
}

func TestCheckButtonsRefusesTooManyBlankLabelsAndBadURLs(t *testing.T) {
	good := telegramrender.Button{Label: "Go", URL: "https://example.com"}
	require.NoError(t, telegramrender.CheckButtons([]telegramrender.Button{good}))
	require.NoError(t, telegramrender.CheckButtons(make8(good)))
	require.ErrorIs(t, telegramrender.CheckButtons(append(make8(good), good)), telegramrender.ErrContent)
	require.ErrorIs(t, telegramrender.CheckButtons([]telegramrender.Button{{Label: " ", URL: good.URL}}), telegramrender.ErrContent)
	require.ErrorIs(t, telegramrender.CheckButtons([]telegramrender.Button{{Label: "x", URL: "ftp://x.test"}}), telegramrender.ErrContent)
	require.ErrorIs(t, telegramrender.CheckButtons([]telegramrender.Button{{Label: "x", URL: "nope"}}), telegramrender.ErrContent)
}

func make8(b telegramrender.Button) []telegramrender.Button {
	out := make([]telegramrender.Button, telegramrender.MaxButtons)
	for i := range out {
		out[i] = b
	}
	return out
}

func TestRewriteLinksReplacesOnlyTheHrefsOfAnchors(t *testing.T) {
	short := func(href string) (string, error) { return "https://s.test/c/" + strings.ToUpper(href[8:9]), nil }
	rendered, err := telegramrender.Render(`<b>x</b> <a href="https://a.test/?q=1&r=2">one</a> &amp; <a href="https://b.test/">two <i>i</i></a>`, nil)
	require.NoError(t, err)

	got, err := telegramrender.RewriteLinks(rendered, short)

	require.NoError(t, err)
	assert.Equal(t, `<b>x</b> <a href="https://s.test/c/A">one</a> &amp; <a href="https://s.test/c/B">two <i>i</i></a>`, got)
}

func TestRewriteLinksWithTheIdentityChangesNothingAndStaysInTheSubset(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		rendered, err := telegramrender.Render(rapid.String().Draw(t, "markup")+`<a href="https://a.test/?x=1&y=2">l</a>`, nil)
		require.NoError(t, err)

		got, err := telegramrender.RewriteLinks(rendered, func(href string) (string, error) { return href, nil })

		require.NoError(t, err)
		assert.Equal(t, rendered, got)
	})
}
