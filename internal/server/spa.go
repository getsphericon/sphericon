package server

import (
	"net/http"

	"github.com/getsphericon/sphericon/internal/server/spa"
)

// spaHandler serves the embedded single-page app (spa.Handler).
//
// It is mounted on the catch-all "/" pattern, so it only sees requests not
// matched by a more specific handler (/site, /api, /collect, the 404ing /auth and /avatar,
// /t.js) — Go 1.22+ ServeMux picks the most specific pattern.
//
// When the SPA is not embedded (default build, no embed_spa tag) it returns a
// hint instead; the frontend is served by Vite in dev.
func spaHandler(settings spa.Settings) http.Handler {
	sub, ok := spaFS()
	if !ok {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusNotImplemented)
			_, _ = w.Write([]byte("frontend not embedded in this build (build with -tags embed_spa); in dev it is served by Vite\n"))
		})
	}
	return spa.Handler(sub, settings)
}
