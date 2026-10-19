package i18n

import "testing"

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
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestConfigureAndTranslate(t *testing.T) {
	// The active locale is process-global; restore English when done so other
	// tests (and the default) are unaffected.
	t.Cleanup(func() { Configure("en") })

	Configure("en")
	if got := T("errors.name_required", nil); got != "name is required" {
		t.Errorf("en errors.name_required = %q", got)
	}

	// Unknown locale falls back to English.
	Configure("de")
	if got := T("errors.name_required", nil); got != "name is required" {
		t.Errorf("unknown-locale fallback = %q", got)
	}
}

func TestTemplateData(t *testing.T) {
	t.Cleanup(func() { Configure("en") })
	Configure("en")
	got := T("email.welcome.body", map[string]any{"Greeting": "Ada"})
	want := "Hi Ada,\n\nWelcome to Sphericon! Your account is ready.\n"
	if got != want {
		t.Errorf("welcome body = %q, want %q", got, want)
	}
}

func TestUnknownMessageReturnsID(t *testing.T) {
	if got := T("errors.does_not_exist", nil); got != "errors.does_not_exist" {
		t.Errorf("missing id = %q, want the id itself", got)
	}
}
