package server

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestBuildIndexShellInjectsLocale(t *testing.T) {
	fs := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(
			`<html lang="{{APP_LOCALE}}"><script>window.__APP_LOCALE__ = '{{APP_LOCALE}}'</script></html>`,
		)},
	}

	shell := buildIndexShell(fs, "ru", true)
	got := string(shell)

	if strings.Contains(got, localeSentinel) {
		t.Fatalf("sentinel %q was not substituted: %s", localeSentinel, got)
	}
	// The value slots become "ru", but the window.__APP_LOCALE__ identifier must
	// survive intact (the sentinel is chosen not to collide with it).
	if !strings.Contains(got, `lang="ru"`) || !strings.Contains(got, `window.__APP_LOCALE__ = 'ru'`) {
		t.Fatalf("locale not injected into both slots: %s", got)
	}
}

func TestBuildIndexShellMissingFile(t *testing.T) {
	if shell := buildIndexShell(fstest.MapFS{}, "en", true); shell != nil {
		t.Fatalf("expected nil shell when index.html is absent, got %q", shell)
	}
}

func TestBuildIndexShellInjectsTheVerificationGate(t *testing.T) {
	fs := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(
			`<script>window.__REQUIRE_EMAIL_VERIFICATION__ = '{{REQUIRE_EMAIL_VERIFICATION}}' !== 'false'</script>`,
		)},
	}

	on := string(buildIndexShell(fs, "en", true))
	off := string(buildIndexShell(fs, "en", false))

	if !strings.Contains(on, `= 'true' !== 'false'`) {
		t.Fatalf("gate on not injected: %s", on)
	}
	if !strings.Contains(off, `= 'false' !== 'false'`) {
		t.Fatalf("gate off not injected: %s", off)
	}
}
