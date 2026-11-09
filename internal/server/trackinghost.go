package server

import (
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/domain/sending/trackingdomains"
)

// trackingHostGate serves a verified Tracking host link paths and the first-party tracker
// only (ADR 0038, 0059, 0071). The host is the configured header when the peer is inside the edge
// ranges, else the connection's Host. A host that is not a Tracking domain in dns_verified or
// serving (in any role: a retired host still serves what was sent on it) goes to next
// unchanged, so the Instance's own hosts serve everything as before; the one exception is a
// header the edge set that names a host which is not verified: it answers 404 instead of
// falling through to the SPA. The Instance's tracker host and a name that cannot be a Tracking
// domain (an IP, a single label, an apex) are never looked up, so the tracker's traffic costs no
// query. domains is the Instance-wide host index, link the link-path handler (/e/, /c/), which
// learns the host's Workspace from auth.HostWorkspace. The well-known probe value is keyed by
// cfg.JWTSecret.
func trackingHostGate(cfg *config.Config, domains *trackingdomains.Module, next, link http.Handler) http.Handler {
	appHost := appHostname(cfg.AppURL)
	trackerHost := appHostname(cfg.TrackerURL)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, fromEdge := requestHost(cfg, r)
		if host == appHost {
			next.ServeHTTP(w, r)
			return
		}
		var (
			ws  int64
			ok  bool
			err error
		)
		if _, candidate := trackingdomains.CanonicalHost(host); candidate && host != trackerHost {
			ws, ok, err = domains.HostWorkspace(r.Context(), host)
		}
		if err != nil || !ok {
			if fromEdge {
				http.NotFound(w, r)
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		switch {
		case r.URL.Path == trackingdomains.ProbePath && (r.Method == http.MethodGet || r.Method == http.MethodHead):
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write([]byte(trackingdomains.ProbeValue(cfg.JWTSecret, host)))
		case isLinkPath(r.URL.Path):
			link.ServeHTTP(w, r.WithContext(auth.WithHostWorkspace(r.Context(), ws)))
		case isTrackerPath(r.URL.Path):
			// The first-party tracker (ADR 0059): the script and the ingestion, which refuses
			// another Workspace's collect key on this host.
			next.ServeHTTP(w, r.WithContext(auth.WithHostWorkspace(r.Context(), ws)))
		default:
			http.NotFound(w, r)
		}
	})
}

// isTrackerPath is the first-party tracker: the script and the collect API.
func isTrackerPath(p string) bool {
	return p == "/t.js" || isCollectPath(p)
}

// isLinkPath is the link surface: /e/ open, click, unsubscribe and confirm, and /c/, the short
// tracked link of a Telegram message.
func isLinkPath(p string) bool {
	for _, prefix := range []string{"/e/o/", "/e/c/", "/e/u/", "/e/confirm/", "/c/"} {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

// requestHost is the lowercase hostname the request was for, without a port, and
// whether it came from the edge's header.
func requestHost(cfg *config.Config, r *http.Request) (string, bool) {
	if cfg.TrackingHostHeader != "" && peerIsEdge(cfg.TrackingEdgeCIDRs, r.RemoteAddr) {
		if v := strings.TrimSpace(r.Header.Get(cfg.TrackingHostHeader)); v != "" {
			return hostname(v), true
		}
	}
	return hostname(r.Host), false
}

func peerIsEdge(cidrs []netip.Prefix, remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	ip = ip.Unmap()
	for _, p := range cidrs {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

func hostname(hostport string) string {
	h := strings.TrimSpace(hostport)
	if sh, _, err := net.SplitHostPort(h); err == nil {
		h = sh
	}
	return strings.ToLower(strings.TrimSuffix(h, "."))
}

func appHostname(appURL string) string {
	h := strings.TrimPrefix(strings.TrimPrefix(appURL, "https://"), "http://")
	if i := strings.IndexAny(h, "/?#"); i >= 0 {
		h = h[:i]
	}
	return hostname(h)
}
