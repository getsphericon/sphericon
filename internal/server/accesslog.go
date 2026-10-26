package server

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/internal/logging"
	"github.com/go-chi/chi/v5/middleware"
)

// accessLog writes one line per finished request: method, path (loggedPath, so a credential in
// it is redacted), status, response bytes and duration, under the request id. The query string,
// headers, body and client address are not logged. It sits outside the timeout and the limiter,
// so a 429, a 413 and a 504 are recorded with the status the client got.
//
// The level follows the status: a 5xx is an error, a 4xx a warning. A success on a public
// endpoint that a page or a mailbox calls constantly (the open pixel, the tracker, collect) is
// debug, and the probes are not logged at all.
func accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)

		status := ww.Status()
		if status == 0 {
			status = http.StatusOK
		}
		logging.FromContext(r.Context()).Log(r.Context(), accessLevel(r.URL.Path, status), "request",
			"method", r.Method,
			"path", loggedPath(r.URL.Path),
			"status", status,
			"bytes", ww.BytesWritten(),
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

func accessLevel(path string, status int) slog.Level {
	switch {
	case status >= http.StatusInternalServerError:
		return slog.LevelError
	case status >= http.StatusBadRequest:
		return slog.LevelWarn
	case path == "/t.js" || strings.HasPrefix(path, "/e/o/") || isCollectPath(path):
		return slog.LevelDebug
	default:
		return slog.LevelInfo
	}
}
