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

	shell := buildIndexShell(fs, "en", true)
	got := string(shell)

	require.NotContainsf(t, got, localeSentinel, "sentinel %q was not substituted: %s", localeSentinel, got)
	// The value slots become "en", but the window.__APP_LOCALE__ identifier must
	// survive intact (the sentinel is chosen not to collide with it).
	require.Contains(t, got, `lang="en"`, "locale not injected into the lang slot")
	require.Contains(t, got, `window.__APP_LOCALE__ = 'en'`, "locale not injected into the script slot")
}

func TestBuildIndexShellMissingFile(t *testing.T) {
	shell := buildIndexShell(fstest.MapFS{}, "en", true)
	require.Nilf(t, shell, "expected nil shell when index.html is absent, got %q", shell)
}

func TestBuildIndexShellInjectsTheVerificationGate(t *testing.T) {
	fs := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(
			`<script>window.__REQUIRE_EMAIL_VERIFICATION__ = '{{REQUIRE_EMAIL_VERIFICATION}}' !== 'false'</script>`,
		)},
	}

	on := string(buildIndexShell(fs, "en", true))
	off := string(buildIndexShell(fs, "en", false))

	require.Contains(t, on, `= 'true' !== 'false'`, "gate on not injected")
	require.Contains(t, off, `= 'false' !== 'false'`, "gate off not injected")
}
