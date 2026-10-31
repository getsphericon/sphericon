package emailrender_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/emailrender"
)

func details(report emailrender.Report, code emailrender.Code) []string {
	return lo.FilterMap(report.Warnings, func(f emailrender.Finding, _ int) (string, bool) {
		return f.Detail, f.Code == code
	})
}

func TestLintReadsEveryFormOfACSSColorForContrast(t *testing.T) {
	for name, body := range map[string]string{
		"a color name":       `<p style="color:yellow">Pale</p>`,
		"hsl()":              `<p style="color:hsl(0, 0%, 93%)">Pale</p>`,
		"an 8-digit hash":    `<p style="color:#eeeeeeff">Pale</p>`,
		"a 4-digit hash":     `<p style="color:#eeef">Pale</p>`,
		"space-separated":    `<p style="color:rgb(238 238 238 / 50%)">Pale</p>`,
		"an upper-case name": `<p style="COLOR:LightYellow">Pale</p>`,
	} {
		t.Run(name, func(t *testing.T) {
			report := emailrender.Lint("Hi", body, nil)
			require.NoError(t, report.Error)
			assert.Len(t, details(report, emailrender.CodeLowContrast), 1, "a pale text on the default white background")
		})
	}
}

func TestLintSkipsATransparentBackground(t *testing.T) {
	report := emailrender.Lint("Hi", `<div style="background:#000000"><p style="background-color:rgba(0, 0, 0, 0);color:#ffffff">Light on dark</p></div>`, nil)

	require.NoError(t, report.Error)
	assert.Empty(t, details(report, emailrender.CodeLowContrast), "the black behind the transparent paragraph is what the text is read against")
}

func TestLintFindsTheColorInsideABackgroundShorthand(t *testing.T) {
	brand := &emailrender.Brand{Colors: []string{"#1a73e8"}, Fonts: []string{"Arial"}, TextColor: "#202124", BackgroundColor: "#ffffff"}

	report := emailrender.Lint("Hi", `<table><tbody><tr><td style="background:url(https://cdn.example.com/a.png) no-repeat center hsl(120, 100%, 25%);color:#ffffff;font-family:Arial, sans-serif">Green</td></tr></tbody></table>`, brand)

	require.NoError(t, report.Error)
	assert.Equal(t, []string{"#008000"}, details(report, emailrender.CodeOffBrandColor))
	assert.Empty(t, details(report, emailrender.CodeOffBrandFont), "a font-family list with a comma is read whole")
}
