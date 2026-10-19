package brandkit_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/brandkit"
	"github.com/getsphericon/sphericon/internal/emailrender"
)

func brandedKit() brandkit.Kit {
	k := brandkit.Defaults()
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
	starters := brandkit.Starters(brandedKit())
	require.Len(t, starters, 4)

	for _, s := range starters {
		t.Run(s.Name, func(t *testing.T) {
			subject := s.Subject
			got, err := emailrender.FromDocument(t.Context(), &subject, s.Doc)
			require.NoError(t, err)

			assert.Equal(t, emailrender.SanitizeHTML(got.Body), got.Body, "the stored body survives the sanitizer")
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
	bodies := map[string]string{}
	for _, s := range starters {
		got, err := emailrender.FromDocument(t.Context(), nil, s.Doc)
		require.NoError(t, err)
		bodies[s.Name] = got.Body
	}
	for _, name := range []string{"Welcome", "One button"} {
		assert.Contains(t, bodies[name], "#ff5500", name)
		assert.Contains(t, bodies[name], "#fefefe", name)
		assert.Contains(t, bodies[name], "border-radius:12px", name)
	}
	assert.Contains(t, bodies["Welcome"], "Georgia, serif")
	assert.Contains(t, bodies["Welcome"], "{{ first_name }}")
}

func TestStartersWithoutALogoOrFooterOmitThem(t *testing.T) {
	for _, s := range brandkit.Starters(brandkit.Defaults()) {
		got, err := emailrender.FromDocument(t.Context(), nil, s.Doc)
		require.NoError(t, err)
		assert.NotContains(t, got.Body, "<img", s.Name)
		assert.NotContains(t, got.Body, "font-size:12px", s.Name+" has no footer")
	}
}
