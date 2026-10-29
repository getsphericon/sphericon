package server

import (
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/getsphericon/sphericon/internal/cleanup"
)

// localeSentinel is the placeholder baked into index.html (see repo-root
// index.html). Both this handler and the Vite dev server substitute the
// instance locale for it at serve time, so the SPA reads window.__APP_LOCALE__
// synchronously at boot without an extra request. It is deliberately distinct
// from the window.__APP_LOCALE__ identifier so substitution can't corrupt it.
const localeSentinel = "{{APP_LOCALE}}"

// verificationSentinel is the placeholder for the email verification gate (ADR 0030):
// "true" or "false", substituted like the locale, so the SPA knows at boot whether
// Sign-up collects a password (window.__REQUIRE_EMAIL_VERIFICATION__).
const verificationSentinel = "{{REQUIRE_EMAIL_VERIFICATION}}"

// sentryDSNSentinel is the placeholder for the browser error reporter's DSN: empty when
// SPHERICON_SENTRY_DSN is unset, which keeps the reporter off (window.__SENTRY_DSN__).
const sentryDSNSentinel = "{{SENTRY_DSN}}"

// spaSettings is what the app shell carries from the Instance's configuration to the SPA at boot.
type spaSettings struct {
	Locale                   string
	RequireEmailVerification bool
	SentryDSN                string
}

// spaHandler serves the embedded single-page app. Real files (the hashed
// /assets/* bundles) are served directly; index.html — served for "/" and as
// the deep-link/refresh fallback — always goes through the injected app shell
// so the instance locale is substituted (the raw file server would serve the
// unsubstituted sentinel).
//
// It is mounted on the catch-all "/" pattern, so it only sees requests not
// matched by a more specific handler (/site, /api, /collect, the 404ing /auth and /avatar,
// /t.js) — Go 1.22+ ServeMux picks the most specific pattern.
//
// When the SPA is not embedded (default build, no embed_spa tag) it returns a
// hint instead; the frontend is served by Vite in dev.
func spaHandler(settings spaSettings) http.Handler {
	sub, ok := spaFS()
	if !ok {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusNotImplemented)
			_, _ = w.Write([]byte("frontend not embedded in this build (build with -tags embed_spa); in dev it is served by Vite\n"))
		})
	}
	return spaFileHandler(sub, settings)
}

// spaFileHandler serves the built frontend from sub (split out of spaHandler so
// the serving logic is testable without an embed_spa build).
func spaFileHandler(sub fs.FS, settings spaSettings) http.Handler {
	// The settings are fixed for the process, so substitute them into the shell once
	// at startup rather than per request. shell is nil if index.html is missing.
	shell := buildIndexShell(sub, settings)
	fileServer := http.FileServerFS(sub)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		// Serve a real, non-index file when it exists; everything else (deep
		// links, refreshes, "/") returns the locale-injected app shell.
		if name != "" && name != "index.html" {
			if f, err := sub.Open(name); err == nil {
				info, statErr := f.Stat()
				cleanup.Close(f, "file")
				if statErr == nil && !info.IsDir() {
					fileServer.ServeHTTP(w, r)
					return
				}
			}
		}
		if shell == nil {
			http.Error(w, "index.html not found", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(shell)
	})
}

// buildIndexShell reads index.html and substitutes the instance locale, the email
// verification gate and the browser reporter's DSN for their sentinels. Returns nil if
// index.html cannot be read.
func buildIndexShell(sub fs.FS, settings spaSettings) []byte {
	b, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		return nil
	}
	return []byte(strings.NewReplacer(
		localeSentinel, settings.Locale,
		verificationSentinel, strconv.FormatBool(settings.RequireEmailVerification),
		sentryDSNSentinel, jsStringContent(settings.SentryDSN),
	).Replace(string(b)))
}

// jsStringContent escapes s for the inside of a single-quoted JavaScript string in an inline
// <script>, so an operator's value can end neither the string nor the script element.
func jsStringContent(s string) string {
	return strings.NewReplacer(
		`\`, `\\`, `'`, `\'`, `<`, `\x3c`, `>`, `\x3e`, "\n", `\n`, "\r", `\r`,
		" ", ` `, " ", ` `,
	).Replace(s)
}
