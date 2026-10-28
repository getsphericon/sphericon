package server

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

func TestBuildIndexShellInjectsLocale(t *testing.T) {
	fs := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(
			`<html lang="{{APP_LOCALE}}"><script>window.__APP_LOCALE__ = '{{APP_LOCALE}}'</script></html>`,
		)},
	}

	shell := buildIndexShell(fs, spaSettings{Locale: "en", RequireEmailVerification: true})
	got := string(shell)

	require.NotContainsf(t, got, localeSentinel, "sentinel %q was not substituted: %s", localeSentinel, got)
	// The value slots become "en", but the window.__APP_LOCALE__ identifier must
	// survive intact (the sentinel is chosen not to collide with it).
	require.Contains(t, got, `lang="en"`, "locale not injected into the lang slot")
	require.Contains(t, got, `window.__APP_LOCALE__ = 'en'`, "locale not injected into the script slot")
}

func TestBuildIndexShellMissingFile(t *testing.T) {
	shell := buildIndexShell(fstest.MapFS{}, spaSettings{Locale: "en", RequireEmailVerification: true})
	require.Nilf(t, shell, "expected nil shell when index.html is absent, got %q", shell)
}

func TestBuildIndexShellInjectsTheVerificationGate(t *testing.T) {
	fs := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(
			`<script>window.__REQUIRE_EMAIL_VERIFICATION__ = '{{REQUIRE_EMAIL_VERIFICATION}}' !== 'false'</script>`,
		)},
	}

	on := string(buildIndexShell(fs, spaSettings{Locale: "en", RequireEmailVerification: true}))
	off := string(buildIndexShell(fs, spaSettings{Locale: "en"}))

	require.Contains(t, on, `= 'true' !== 'false'`, "gate on not injected")
	require.Contains(t, off, `= 'false' !== 'false'`, "gate off not injected")
}

func TestBuildIndexShellInjectsTheSentryDSN(t *testing.T) {
	fs := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(`<script>window.__SENTRY_DSN__ = '{{SENTRY_DSN}}'</script>`)},
	}

	unset := string(buildIndexShell(fs, spaSettings{}))
	set := string(buildIndexShell(fs, spaSettings{SentryDSN: "https://key@o1.ingest.sentry.io/2"}))

	require.Contains(t, unset, `__SENTRY_DSN__ = ''`, "an unset DSN must leave the slot empty")
	require.Contains(t, set, `__SENTRY_DSN__ = 'https://key@o1.ingest.sentry.io/2'`, "the DSN was not injected")
}

func TestBuildIndexShellEscapesTheSentryDSNForAScriptString(t *testing.T) {
	fs := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(`<script>window.__SENTRY_DSN__ = '{{SENTRY_DSN}}'</script>`)},
	}

	got := string(buildIndexShell(fs, spaSettings{SentryDSN: `x'</script><b>\`}))

	require.NotContains(t, got, "</script><b>", "the value ended the script element")
	require.Contains(t, got, `__SENTRY_DSN__ = 'x\'\x3c/script\x3e\x3cb\x3e\\'`, "the value was not escaped for a JS string")
}
