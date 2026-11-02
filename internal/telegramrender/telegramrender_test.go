package telegramrender_test

import (
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
