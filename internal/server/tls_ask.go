package server

import (
	"net/http"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/internal/sendingdomains"
)

// tlsAskHandler answers the edge's on-demand TLS question (ADR 0038): GET ?domain=<host>
// is 200 only when the host is the Tracking domain host (track.<domain>) of a Sending
// domain whose tracking DNS is verified (dns_verified or serving), 404 for anything
// else (pending, unknown, malformed). It reads the Instance-wide host index, so it holds
// the raw client and is mounted only on the internal listener.
func tlsAskHandler(client *ent.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		domain, ok := trackingDomainOf(r.URL.Query().Get("domain"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		found, err := client.SendingDomain.Query().
			Where(
				sendingdomain.Domain(domain),
				sendingdomain.TrackingStateIn(sendingdomain.TrackingStateDNSVerified, sendingdomain.TrackingStateServing),
			).
			Exist(r.Context())
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

// trackingDomainOf is the Sending domain a Tracking domain host belongs to: the host
// without its track. label, lowercased. ok is false for anything that is not such a host.
func trackingDomainOf(host string) (string, bool) {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	domain, found := strings.CutPrefix(host, sendingdomains.TrackingHost(""))
	if !found || domain == "" || strings.ContainsAny(domain, "/:@ ?#") {
		return "", false
	}
	return domain, true
}
