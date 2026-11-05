// Package i18n localizes sphericon's own system-generated text — the platform
// (transactional) emails and the API validation messages. It is NOT for
// recipient-facing campaign content, which is authored per workspace.
//
// Locale is a property of the deployment (a SaaS instance runs per country),
// fixed at process start via SPHERICON_APP_LOCALE — there is no per-request or per-user
// locale. So the translator is a process-global, configured once at boot
// (Configure) and read-only thereafter. English is the only catalog.
package i18n

import (
	"embed"
	"encoding/json"
	"slices"
	"sync"

	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

//go:embed locales/*.json
var localesFS embed.FS

// supported lists the locales shipped with the binary: English only. Kept in lockstep with the
// frontend's SUPPORTED_LOCALES (src/i18n.ts) and the Vite dev plugin.
var supported = []string{"en"}

var (
	bundle = newBundle()
	mu     sync.RWMutex
	// active is English until Configure runs. This keeps tests (which never set
	// SPHERICON_APP_LOCALE) asserting the English strings, and makes T safe before boot.
	active = goi18n.NewLocalizer(bundle, "en")
)

func newBundle() *goi18n.Bundle {
	b := goi18n.NewBundle(language.English)
	b.RegisterUnmarshalFunc("json", json.Unmarshal)
	for _, l := range supported {
		// en is both the default and the fallback, so every catalog must load;
		// a missing/broken embedded file is a build-time bug, not a runtime one.
		if _, err := b.LoadMessageFileFS(localesFS, "locales/"+l+".json"); err != nil {
			panic("i18n: load catalog " + l + ": " + err.Error())
		}
	}
	return b
}

// Supported returns the locales the binary ships with (copy; safe to mutate).
func Supported() []string {
	return slices.Clone(supported)
}

// Normalize coerces an arbitrary locale string to a supported one, falling back
// to "en" for anything unknown. Used by config and the SPA locale injection so
// there is a single definition of "which locales are valid".
func Normalize(locale string) string {
	if slices.Contains(supported, locale) {
		return locale
	}
	return "en"
}

// Configure sets the active locale for the process. Call once at startup with
// cfg.Locale (app.New). Unknown locales fall back to English. The active
// localizer always lists "en" as the final fallback, so a partially translated
// catalog still renders — missing keys come through in English.
func Configure(locale string) {
	l := Normalize(locale)
	mu.Lock()
	active = goi18n.NewLocalizer(bundle, l, "en")
	mu.Unlock()
}

// ID names a message of the catalog. Only the generated ids (ids_gen.go, from locales/en.json)
// make one, so a call can only name a message the catalog has; a message chosen at runtime is
// picked from a map of ids, never built from a key.
type ID struct{ key string }

// Key is the id's key in the catalog (EmailPasswordResetSubject).
func (id ID) Key() string { return id.key }

// T localizes a message, interpolating data (Go text/template syntax, e.g. {{.Name}}) when
// provided. A message the active locale lacks comes through in English.
func T(id ID, data map[string]any) string {
	mu.RLock()
	loc := active
	mu.RUnlock()
	msg, err := loc.Localize(&goi18n.LocalizeConfig{MessageID: id.key, TemplateData: data})
	if err != nil {
		return id.key
	}
	return msg
}
