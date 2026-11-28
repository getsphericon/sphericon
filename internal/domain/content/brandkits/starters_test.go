package brandkits_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/content/brandkits"
	"github.com/getsphericon/sphericon/internal/kernel/emailrender"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func brandedKit() brandkits.Kit {
	k := brandkits.Defaults()
	k.AccentColor = "#ff5500"
	k.TextColor = "#102030"
	k.BackgroundColor = "#f0f4f8"
	k.HeadingFont = "Georgia, serif"
	k.BodyFont = "Verdana, sans-serif"
	k.LogoURL = "https://cdn.example.com/logo.png"
	k.ButtonTextColor = "#fefefe"
	k.ButtonRadius = 12
	k.FooterText = "Acme Inc, 1 Main St"
	return k
}

func TestStartersCarryTheBrandKitInTheirStoredBodies(t *testing.T) {
	starters := brandkits.Starters(brandedKit())
	require.Len(t, starters, 4)

	bodies := map[string]string{}
	for _, s := range starters {
		t.Run(s.Name, func(t *testing.T) {
			subject := s.Subject
			got, err := emailrender.FromDocument(t.Context(), testhelper.RealRenderer(), &subject, s.Doc)
			require.NoError(t, err)
			bodies[s.Name] = got.Body

			kit := brandedKit()
			kit.Saved = true
			report := emailrender.Lint(subject, got.Body, kit.Brand())
			require.NoError(t, report.Error)
			for _, w := range report.Warnings {
				assert.Equal(t, emailrender.CodeLowContrast, w.Code, "a starter is made of its own Brand kit: only the test kit's orange button can warn: %v", w)
			}
			for _, brand := range []string{
				"#f0f4f8", "#102030", "Verdana, sans-serif", "https://cdn.example.com/logo.png", "Acme Inc, 1 Main St",
			} {
				assert.Contains(t, got.Body, brand)
			}
		})
	}

	// The two starters with a button carry the button style; the heading starters the heading font.
	for _, name := range []string{"Welcome", "One button"} {
		assert.Contains(t, bodies[name], "#ff5500", name)
		assert.Contains(t, bodies[name], "#fefefe", name)
		assert.Contains(t, bodies[name], "border-radius:12px", name)
	}
	assert.Contains(t, bodies["Welcome"], "Georgia, serif")
	assert.Contains(t, bodies["Welcome"], "{{ first_name }}")
}

// ADR 0188: a check that fires on what the app itself produces is wrong.
func TestStartersHoldNoHiddenTextOrInvisibleCharacters(t *testing.T) {
	for _, s := range append(brandkits.Starters(brandedKit()), brandkits.Starters(brandkits.Defaults())...) {
		t.Run(s.Name, func(t *testing.T) {
			subject := s.Subject
			got, err := emailrender.FromDocument(t.Context(), testhelper.RealRenderer(), &subject, s.Doc)
			require.NoError(t, err)

			report := emailrender.Lint(subject, got.Body, nil)

			require.NoError(t, report.Error)
			assert.NotContains(t, codesOf(report), emailrender.CodeHiddenText)
			assert.NotContains(t, codesOf(report), emailrender.CodeHiddenCharacters)
		})
	}
}

func codesOf(report emailrender.Report) []emailrender.Code {
	return lo.Map(report.Warnings, func(w emailrender.Finding, _ int) emailrender.Code { return w.Code })
}

func TestStartersWithoutALogoOrFooterOmitThem(t *testing.T) {
	for _, s := range brandkits.Starters(brandkits.Defaults()) {
		got, err := emailrender.FromDocument(t.Context(), testhelper.RealRenderer(), nil, s.Doc)
		require.NoError(t, err)
		assert.NotContains(t, got.Body, "<img", s.Name)
		assert.NotContains(t, got.Body, "font-size:12px", s.Name+" has no footer")
	}
}
