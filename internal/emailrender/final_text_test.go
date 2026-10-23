package emailrender_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/emailrender"
)

// hidingStyles are the inline styles that take an element out of the reader's view.
var hidingStyles = []string{
	"display:none",
	"display: none !important",
	"visibility:hidden",
	"mso-hide:all",
	"opacity:0",
	"font-size:0",
	"font-size: 0px",
	"max-height:0",
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
