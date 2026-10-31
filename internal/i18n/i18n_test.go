package i18n_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/i18n"
)

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"en":      "en",
		"":        "en",
		"de":      "en",
		"EN":      "en", // case-sensitive: only exact matches are supported
		"ru":      "en",
		"ru-RU":   "en",
		"garbage": "en",
	}
	for in, want := range cases {
		got := i18n.Normalize(in)
		assert.Equalf(t, want, got, "Normalize(%q) = %q, want %q", in, got, want)
	}
}

func TestConfigureAndTranslate(t *testing.T) {
	// The active locale is process-global; restore English when done so other
	// tests (and the default) are unaffected.
	t.Cleanup(func() { i18n.Configure("en") })

	i18n.Configure("en")
	got := i18n.T(i18n.ErrorsNameRequired, nil)
	assert.Equalf(t, "name is required", got, "en errors.name_required = %q", got)

	// Unknown locale falls back to English.
	i18n.Configure("de")
	got = i18n.T(i18n.ErrorsNameRequired, nil)
	assert.Equalf(t, "name is required", got, "unknown-locale fallback = %q", got)
}

func TestTemplateData(t *testing.T) {
	t.Cleanup(func() { i18n.Configure("en") })
	i18n.Configure("en")
	got := i18n.T(i18n.EmailWelcomeBody, map[string]any{"Greeting": "Ada"})
	want := "Hi Ada,\n\nWelcome to Sphericon! Your account is ready.\n"
	assert.Equalf(t, want, got, "welcome body = %q, want %q", got, want)
}

func TestAnIDNamesItsKey(t *testing.T) {
	assert.Equal(t, "email.welcome.body", i18n.EmailWelcomeBody.Key())
}
