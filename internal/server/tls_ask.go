package server

import (
	"net/http"

	"github.com/getsphericon/sphericon/ent"
)

// tlsAskHandler answers the edge's on-demand TLS question (ADR 0038): GET ?domain=<host>
// is 200 only when the host is a Tracking domain whose DNS is verified (dns_verified or
// serving, retired ones included), 404 for anything else (pending, unknown, malformed). It reads the Instance-wide host index, so it holds
// the raw client and is mounted only on the internal listener.
func tlsAskHandler(client *ent.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, found, err := verifiedTrackingHost(r.Context(), client, r.URL.Query().Get("domain"))
		if err != nil {
			http.Error(w, "lookup failed", http.StatusServiceUnavailable)
			return
		}
		if !found {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
}
