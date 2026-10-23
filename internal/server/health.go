package server

import (
	"context"
	"database/sql"
	"net/http"
	"time"
)

// healthzHandler is a liveness probe: it returns 200 as long as the process is
// serving, without touching any dependency.
func healthzHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(statusOK))
	})
}

// statusOK is the body of a passing probe, a JSON object and its newline.
const statusOK = "{\"status\":\"ok\"}\n"

// readyzHandler is a readiness probe: it reports 200 only when the database is
// reachable, otherwise 503 — so orchestrators stop routing traffic to a replica
// that cannot serve requests.
func readyzHandler(db *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			writeProblem(w, http.StatusServiceUnavailable, "database unreachable")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(statusOK))
	})
}
