package emailrender_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/kernel/emailrender"
)

// hidingStyles are the inline styles that take an element out of the reader's view.
var hidingStyles = []string{
	"display:none",
	"display: none !important",
	"visibility:hidden",
	"mso-hide:all",
	"opacity:0",
	"max-height:0;overflow:hidden",
	"color:red;MAX-HEIGHT : 0px;overflow:hidden",
}

func TestFinalHTMLToTextDropsHiddenElements(t *testing.T) {
	for _, style := range hidingStyles {
		t.Run(style, func(t *testing.T) {
			text := emailrender.FinalHTMLToText(`<div style="` + style + `">SECRET</div><p>Shown</p>`)

			assert.NotContains(t, text, "SECRET")
			assert.Contains(t, text, "Shown")
		})
	}
}

func TestFinalHTMLToTextKeepsVisibleStyledElements(t *testing.T) {
	text := emailrender.FinalHTMLToText(`<p style="font-size:16px;opacity:0.5;max-height:40px">Visible</p>`)

	assert.Contains(t, text, "Visible")
}

// A layout table sets font-size:0 to close the gaps between its columns, and the text inside
// sets its own size: it is the body of the mail, not hidden. A zero max-height that clips
// nothing hides nothing either.
func TestFinalHTMLToTextKeepsTextInsideAZeroSizeWrapper(t *testing.T) {
	for _, style := range []string{"font-size:0", "font-size: 0px", "max-height:0"} {
		t.Run(style, func(t *testing.T) {
			text := emailrender.FinalHTMLToText(`<div style="` + style + `"><p style="font-size:16px">Body</p></div>`)

			assert.Contains(t, text, "Body")
		})
	}
}

func TestFinalHTMLToTextDropsStyleAndHeadContent(t *testing.T) {
	text := emailrender.FinalHTMLToText(`<html><head><title>HEADTITLE</title><style>p{color:red}</style></head>` +
		`<body><style>a{color:blue}</style><p>Shown</p></body></html>`)

	assert.NotContains(t, text, "HEADTITLE")
	assert.NotContains(t, text, "color:")
	assert.Contains(t, text, "Shown")
}

func TestFinalHTMLToTextRendersALinkAsItsTextAndURL(t *testing.T) {
	text := emailrender.FinalHTMLToText(`<a href="https://example.com/x">Click</a>`)

	assert.Contains(t, text, "Click")
	assert.Contains(t, text, "https://example.com/x")
}

// The editor's export carries no whitespace between its blocks, and a hand-written body need not
// either: a text part reads the same words whether the markup is indented or compact, and a link's
// URL never runs into what follows it.
func TestTextPartSeparatesBlocksWithoutWhitespaceInTheMarkup(t *testing.T) {
	compact := `<table><tr><td><a href="https://a.example">Get started</a></td></tr></table>` +
		`<table><tr><td><a href="https://b.example">Read more</a></td></tr></table>` +
		`<table><tr><td>Left</td><td>Right</td></tr></table><div>Above</div><div>Below</div><hr><p>tail</p>`
	indented := `<table>
  <tr><td><a href="https://a.example">Get started</a></td></tr>
</table>
<table>
  <tr><td><a href="https://b.example">Read more</a></td></tr>
</table>
<table>
  <tr>
    <td>Left</td>
    <td>Right</td>
  </tr>
</table>
<div>Above</div>
<div>Below</div>
<hr>
<p>tail</p>`

	for _, tc := range []struct {
		name   string
		derive func(string) string
		links  []string
	}{
		{"HTMLToText", emailrender.HTMLToText, []string{"https://a.example", "https://b.example"}},
		{"FinalHTMLToText", emailrender.FinalHTMLToText, []string{"<https://a.example>", "<https://b.example>"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.Fields(tc.derive(compact))

			assert.Equal(t, strings.Fields(tc.derive(indented)), got)
			for _, word := range append(tc.links, "Left", "Right", "Above", "Below", "tail") {
				assert.Contains(t, got, word)
			}
		})
	}
}

func TestFinalHTMLToTextNeverShowsHiddenText(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		style := rapid.SampledFrom(hidingStyles).Draw(t, "style")
		hidden := rapid.StringMatching(`[A-Z]{12}`).Draw(t, "hidden")
		shown := rapid.StringMatching(`[a-z]{12}`).Draw(t, "shown")
		tag := rapid.SampledFrom([]string{"div", "span", "p"}).Draw(t, "tag")
		inner := rapid.SampledFrom([]string{"", "<b>", "<a href=\"https://example.com\">"}).Draw(t, "wrap")

		markup := "<" + tag + ` style="` + style + `">` + inner + hidden + "</" + tag + "><p>" + shown + "</p>"
		text := emailrender.FinalHTMLToText(markup)

		assert.NotContains(t, text, hidden)
		assert.Contains(t, text, shown)
	})
}

const hiddenMarker = "HIDDENMARKER"

func FuzzFinalHTMLToText(f *testing.F) {
	f.Add(`<p>Hi</p><div style="display:none">x</div><a href="https://example.com">a</a>`)
	f.Add(`<style>p{}</style><img src="x" style="opacity:0">`)
	f.Add(`<div style=";;::"`)
	f.Fuzz(func(t *testing.T, markup string) {
		// Whatever the bytes, the derivation returns (no panic) and never leaves a hidden marker in.
		text := emailrender.FinalHTMLToText(`<div style="display:none">` + hiddenMarker + `</div>` + markup)

		assert.NotContains(t, text, hiddenMarker)
	})
}
