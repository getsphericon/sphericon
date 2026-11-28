package emailrender_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/kernel/emailrender"
)

func elements(report emailrender.Report, code emailrender.Code) []string {
	return lo.FilterMap(report.Warnings, func(f emailrender.Finding, _ int) (string, bool) {
		return f.Element, f.Code == code
	})
}

func TestLintWarnsOnAParagraphHiddenMidBody(t *testing.T) {
	report := emailrender.Lint("Hi", `<p>Hello</p><p style="display:none">Claim your prize</p><p>Bye</p>`, nil)

	require.NoError(t, report.Error)
	assert.Equal(t, []string{`<p> "Claim your prize"`}, elements(report, emailrender.CodeHiddenText))
}

// editorPreheader is the preview text as the editor's export writes it (react-email's Preview): a
// <title>, then a hidden div of the text and a child div that pads it to 200 characters with
// invisible characters, so that the mail client shows no body text after it.
func editorPreheader(text string) string {
	return `<title>` + text + `</title>` +
		`<div style="display:none;overflow:hidden;line-height:1px;opacity:0;max-height:0;max-width:0" data-skip-in-text="true">` +
		text + `<div>` + strings.Repeat("\u00a0\u200c\u200b\u200d\ufeff", 20) + `</div></div>`
}

func TestLintLeavesThePreheaderAndItsPaddingAlone(t *testing.T) {
	report := emailrender.Lint("Hi", editorPreheader("Sale ends today")+`<p>Hello</p>`, nil)

	require.NoError(t, report.Error)
	assert.Empty(t, report.Warnings)
}

func TestLintWarnsOnHiddenTextAfterThePreheader(t *testing.T) {
	report := emailrender.Lint("Hi", editorPreheader("Sale ends today")+`<p>Hello</p><div style="display:none">Claim your prize</div>`, nil)

	require.NoError(t, report.Error)
	assert.Equal(t, []string{`<div> "Claim your prize"`}, elements(report, emailrender.CodeHiddenText))
}

func TestLintTakesAHiddenElementAfterAnImageForHiddenText(t *testing.T) {
	report := emailrender.Lint("Hi", `<img src="https://cdn.acme.com/logo.png" alt="Acme"><div style="display:none">Claim your prize</div>`, nil)

	require.NoError(t, report.Error)
	assert.Equal(t, []string{`<div> "Claim your prize"`}, elements(report, emailrender.CodeHiddenText))
}

func TestLintWarnsOnTextInsideAHiddenWrapper(t *testing.T) {
	report := emailrender.Lint("Hi", `<p>Hello</p><table style="visibility:hidden"><tr><td><p>Claim your prize</p></td></tr></table>`, nil)

	require.NoError(t, report.Error)
	assert.Equal(t, []string{"Claim your prize"}, details(report, emailrender.CodeHiddenText))
}

func TestLintTakesAHiddenElementOfInvisibleCharactersForNoText(t *testing.T) {
	report := emailrender.Lint("Hi", `<p>Hello</p><div style="display:none">&#847;&zwnj;&nbsp;&#8203;&zwnj;&nbsp;</div><img src="https://cdn.acme.com/pixel.gif" alt="" style="display:none">`, nil)

	require.NoError(t, report.Error)
	assert.Empty(t, elements(report, emailrender.CodeHiddenText), "preheader padding and an open pixel hide no words")
}

const mobileStyle = `<style>@media only screen and (max-width:480px){.mobile-only{display:block !important;max-height:none !important}}</style>`

func TestLintLeavesABlockAMediaQueryShowsAlone(t *testing.T) {
	report := emailrender.Lint("Hi", mobileStyle+`<p>Hello</p><div class="cta mobile-only" style="display:none;max-height:0;overflow:hidden"><p>Tap to call us</p></div>`, nil)

	require.NoError(t, report.Error)
	assert.Empty(t, elements(report, emailrender.CodeHiddenText))
}

func TestLintWarnsOnABlockAMediaQueryKeepsHidden(t *testing.T) {
	for name, style := range map[string]string{
		"the rule keeps it hidden":   `<style>@media (max-width:480px){.mobile-only{display:none}}</style>`,
		"the rule is outside @media": `<style>.mobile-only{display:block}</style>`,
	} {
		t.Run(name, func(t *testing.T) {
			report := emailrender.Lint("Hi", style+`<p>Hello</p><div class="mobile-only" style="display:none">Claim your prize</div>`, nil)

			require.NoError(t, report.Error)
			assert.Equal(t, []string{`<div> "Claim your prize"`}, elements(report, emailrender.CodeHiddenText))
		})
	}
}

func TestLintWarnsOnAZeroWidthSpaceInsideAWordOfTheSubject(t *testing.T) {
	report := emailrender.Lint("Fr\u200bee gift", `<p>Hello</p>`, nil)

	require.NoError(t, report.Error)
	assert.Equal(t, []string{"subject \"Fr\\u200bee gift\""}, elements(report, emailrender.CodeHiddenCharacters))
	assert.Equal(t, []string{"U+200B"}, details(report, emailrender.CodeHiddenCharacters))
}

func TestLintWarnsOnInvisibleCharactersInTheVisibleText(t *testing.T) {
	for name, tc := range map[string]struct{ body, detail string }{
		"a lone tag character":       {"<p>Hello \U000E0041</p>", "U+E0041"},
		"a flag left unterminated":   {"<p>Go \U0001F3F4\U000E0067\U000E0062</p>", "U+E0067 U+E0062"},
		"a word joiner":              {"<p>Fr\u2060ee</p>", "U+2060"},
		"a zero-width no-break":      {"<p>Fr&#65279;ee</p>", "U+FEFF"},
		"a zero-width space in code": {"<pre><code>pip\u200binstall</code></pre>", "U+200B"},
	} {
		t.Run(name, func(t *testing.T) {
			report := emailrender.Lint("Hi", tc.body, nil)

			require.NoError(t, report.Error)
			assert.Equal(t, []string{tc.detail}, details(report, emailrender.CodeHiddenCharacters))
		})
	}
}

func TestLintLeavesEmojiAndTheJoinersOfScriptsAlone(t *testing.T) {
	for name, text := range map[string]string{
		"a ZWJ emoji":             "Coding \U0001F469\u200d\U0001F4BB",
		"the England flag":        "Go \U0001F3F4\U000E0067\U000E0062\U000E0065\U000E006E\U000E0067\U000E007F",
		"Persian with a ZWNJ":     "\u0645\u06cc\u200c\u062e\u0648\u0627\u0647\u0645",
		"Hebrew with an RLM":      "\u05e9\u05dc\u05d5\u05dd\u200f!",
		"the editor's code space": "pip\u00a0\u200d\u200binstall",
	} {
		t.Run(name, func(t *testing.T) {
			report := emailrender.Lint(text, "<p>"+text+"</p>", nil)

			require.NoError(t, report.Error)
			assert.Empty(t, report.Warnings)
		})
	}
}

// codeDoc is the golden code.json without its merge tags, which a code block breaks: Liquid
// cannot read a tag whose spaces the code block wrote as U+00A0 U+200D U+200B.
const codeDoc = `{"type":"doc","content":[
	{"type":"codeBlock","attrs":{"language":"bash","theme":"dracula"},"content":[{"type":"text","text":"pip install sphericon"}]},
	{"type":"codeBlock","content":[{"type":"text","text":"const sent = await client.send({ to: 'a@example.com' }) // 202"}]}]}`

// ADR 0188: a check that fires on what the editor itself exports is wrong. Each document of the
// editor's golden exports is linted as it is saved: its body as FromDocument stores it.
func TestLintFindsNoHiddenContentInTheEditorsExports(t *testing.T) {
	docs := map[string][]byte{"code": []byte(codeDoc)}
	for _, name := range []string{"layout", "newsletter"} {
		doc, err := os.ReadFile(filepath.Join("..", "emaildoc", "testdata", "docs", name+".json"))
		require.NoError(t, err)
		docs[name] = doc
	}

	for name, doc := range docs {
		t.Run(name, func(t *testing.T) {
			content, err := emailrender.FromDocument(t.Context(), renderer, nil, doc)
			require.NoError(t, err)

			report := emailrender.Lint("Hi", content.Body, nil)

			require.NoError(t, report.Error)
			assert.Empty(t, elements(report, emailrender.CodeHiddenText))
			assert.Empty(t, elements(report, emailrender.CodeHiddenCharacters))
		})
	}
}
