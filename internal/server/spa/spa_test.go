package spa_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/server/spa"
)

// en is the settings of an English Instance that requires email verification and reports nothing.
var en = spa.Settings{Locale: "en", RequireEmailVerification: true}

// frontend is a built frontend whose index.html is shell.
func frontend(shell string) fstest.MapFS {
	return fstest.MapFS{
		"index.html":    {Data: []byte(shell)},
		"assets/app.js": {Data: []byte(`console.log(1)`)},
	}
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
	return rec
}

// shellOf is the app shell the handler serves for "/" from index.html shell.
func shellOf(t *testing.T, shell string, settings spa.Settings) string {
	t.Helper()
	rec := get(t, spa.Handler(frontend(shell), settings), "/")
	require.Equal(t, http.StatusOK, rec.Code)
	return rec.Body.String()
}

func TestARealFileIsServedAsItIs(t *testing.T) {
	rec := get(t, spa.Handler(frontend(`<html></html>`), en), "/assets/app.js")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "console.log(1)", rec.Body.String())
}

// "/", index.html, a deep link and a directory all return the shell, with the locale in its
// value slots and the window.__APP_LOCALE__ identifier intact.
func TestEveryOtherPathIsTheShellWithTheLocale(t *testing.T) {
	h := spa.Handler(frontend(`<html lang="{{APP_LOCALE}}"><script>window.__APP_LOCALE__ = '{{APP_LOCALE}}'</script></html>`), en)

	for _, path := range []string{"/", "/index.html", "/workspaces/AcmeWs01/contacts", "/assets"} {
		rec := get(t, h, path)

		require.Equal(t, http.StatusOK, rec.Code, path)
		assert.Equal(t, `<html lang="en"><script>window.__APP_LOCALE__ = 'en'</script></html>`, rec.Body.String(), path)
		assert.Equal(t, "text/html; charset=utf-8", rec.Header().Get("Content-Type"), path)
	}
}

func TestAFrontendWithoutAnIndexIsAnError(t *testing.T) {
	rec := get(t, spa.Handler(fstest.MapFS{"assets/app.js": {Data: []byte("x")}}, en), "/deep/link")

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestTheShellCarriesTheVerificationGate(t *testing.T) {
	const shell = `<script>window.__REQUIRE_EMAIL_VERIFICATION__ = '{{REQUIRE_EMAIL_VERIFICATION}}' !== 'false'</script>`

	on := shellOf(t, shell, spa.Settings{Locale: "en", RequireEmailVerification: true})
	off := shellOf(t, shell, spa.Settings{Locale: "en"})

	assert.Contains(t, on, `= 'true' !== 'false'`)
	assert.Contains(t, off, `= 'false' !== 'false'`)
}

func TestTheShellCarriesTheSentryDSN(t *testing.T) {
	const shell = `<script>window.__SENTRY_DSN__ = '{{SENTRY_DSN}}'</script>`

	unset := shellOf(t, shell, spa.Settings{})
	set := shellOf(t, shell, spa.Settings{SentryDSN: "https://key@o1.ingest.sentry.io/2"})

	assert.Contains(t, unset, `__SENTRY_DSN__ = ''`, "an unset DSN leaves the slot empty")
	assert.Contains(t, set, `__SENTRY_DSN__ = 'https://key@o1.ingest.sentry.io/2'`)
}

func TestTheShellEscapesTheSentryDSNForAScriptString(t *testing.T) {
	got := shellOf(t, `<script>window.__SENTRY_DSN__ = '{{SENTRY_DSN}}'</script>`, spa.Settings{SentryDSN: `x'</script><b>\`})

	assert.NotContains(t, got, "</script><b>", "the value ended the script element")
	assert.Contains(t, got, `__SENTRY_DSN__ = 'x\'\x3c/script\x3e\x3cb\x3e\\'`)
}
