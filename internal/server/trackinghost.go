package server

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/sendingdomains"
	"github.com/getsphericon/sphericon/internal/trackingdomain"
)

type trackingHostKey struct{}

// trackingHost is the verified Tracking domain a request arrived on.
type trackingHost struct {
	host        string
	workspaceID int64
}

// trackingHostFrom returns the Tracking domain the request was served on, if any.
func trackingHostFrom(ctx context.Context) (trackingHost, bool) {
	h, ok := ctx.Value(trackingHostKey{}).(trackingHost)
	return h, ok
}

// trackingHostGate serves a verified Tracking host link paths only (ADR 0038). The host
// is the configured header when the peer is inside the edge ranges, else the connection's
// Host. A host that is not track.<a Sending domain in dns_verified or serving> goes to
// next unchanged, so the Instance's own host serves everything as before; the one
// exception is a header the edge set that names a customer host which is not verified:
// it answers 404 instead of falling through to the SPA. link is the link-path handler
// (/e/). The well-known probe value is keyed by cfg.JWTSecret.
func trackingHostGate(cfg *config.Config, client *ent.Client, next, link http.Handler) http.Handler {
	appHost := appHostname(cfg.AppURL)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, fromEdge := requestHost(cfg, r)
		if host == appHost {
			next.ServeHTTP(w, r)
			return
		}
		ws, ok, err := verifiedTrackingHost(r.Context(), client, host)
		if err != nil || !ok {
			if fromEdge {
				http.NotFound(w, r)
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		switch {
		case r.URL.Path == trackingdomain.ProbePath && (r.Method == http.MethodGet || r.Method == http.MethodHead):
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write([]byte(trackingdomain.ProbeValue(cfg.JWTSecret, host)))
		case isLinkPath(r.URL.Path):
			ctx := context.WithValue(r.Context(), trackingHostKey{}, trackingHost{host: host, workspaceID: ws})
			link.ServeHTTP(w, r.WithContext(ctx))
		default:
			http.NotFound(w, r)
		}
	})
}

// isLinkPath is the link surface of /e/: open, click, unsubscribe and confirm.
func isLinkPath(p string) bool {
	for _, prefix := range []string{"/e/o/", "/e/c/", "/e/u/", "/e/confirm/"} {
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

// verifiedTrackingHost resolves host to its Workspace when it is the Tracking domain of
// a kept (not archived) Sending domain in dns_verified or serving. The partial unique index makes it one row.
// ok is false, with a nil error, for a host that is not one; err is a failed lookup.
func verifiedTrackingHost(ctx context.Context, client *ent.Client, host string) (workspaceID int64, ok bool, err error) {
	domain, isHost := sendingdomains.DomainOfTrackingHost(host)
	if !isHost {
		return 0, false, nil
	}
	row, err := client.SendingDomain.Query().
		Where(sendingdomain.Domain(domain), sendingdomain.DiscardedAtIsNil(), sendingdomain.HasWorkspaceWith(accounts.Live()), sendingdomain.TrackingStateIn(sendingdomain.TrackingStateDNSVerified, sendingdomain.TrackingStateServing)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return row.WorkspaceID, true, nil
}
