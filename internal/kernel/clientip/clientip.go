// Package clientip carries the best-effort client address of a request through the
// context, so ogen handlers (which never see the *http.Request) can record it as
// proof, e.g. on a double-opt-in confirmation, and the rate limits can key on it.
package clientip

import (
	"context"
	"net"
	"net/http"
	"net/netip"

	"github.com/realclientip/realclientip-go"
)

type (
	ctxKey          struct{}
	userAgentCtxKey struct{}
)

// Resolver finds a request's client address behind the Instance's reverse proxies (ADR 0175).
// X-Forwarded-For is read only when the connection comes from a trusted proxy, and then its
// rightmost address outside the trusted ranges is the client's: a client that reaches the binary
// directly cannot name another address, and any number of trusted proxies in front of it are
// skipped rather than taken for the client.
type Resolver struct {
	trusted []netip.Prefix
	xff     realclientip.RightmostTrustedRangeStrategy
}

// New builds a Resolver that trusts the proxies in trusted.
func New(trusted []netip.Prefix) *Resolver {
	ranges := make([]net.IPNet, 0, len(trusted))
	for _, p := range trusted {
		p = p.Masked()
		ranges = append(ranges, net.IPNet{IP: p.Addr().AsSlice(), Mask: net.CIDRMask(p.Bits(), p.Addr().BitLen())})
	}
	xff, err := realclientip.NewRightmostTrustedRangeStrategy("X-Forwarded-For", ranges)
	if err != nil {
		panic(err) // a static header name: cannot fail
	}
	return &Resolver{trusted: trusted, xff: xff}
}

// FromRequest returns the client address of r: the connection's own, or, when that is a trusted
// proxy, the rightmost address of X-Forwarded-For no trusted proxy holds.
func (c *Resolver) FromRequest(r *http.Request) string {
	peer := r.RemoteAddr
	if host, _, err := net.SplitHostPort(peer); err == nil {
		peer = host
	}
	if !c.trusts(peer) {
		return peer
	}
	if ip := c.xff.ClientIP(r.Header, r.RemoteAddr); ip != "" {
		return ip
	}
	return peer
}

func (c *Resolver) trusts(host string) bool {
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	ip = ip.Unmap()
	for _, p := range c.trusted {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// Middleware stores FromRequest and the User-Agent in the request context for
// FromContext and UserAgentFromContext.
func (c *Resolver) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), ctxKey{}, c.FromRequest(r))
		ctx = context.WithValue(ctx, userAgentCtxKey{}, r.UserAgent())
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// UserAgentFromContext returns the request's User-Agent stored by Middleware, or ""
// outside a request.
func UserAgentFromContext(ctx context.Context) string {
	ua, _ := ctx.Value(userAgentCtxKey{}).(string)
	return ua
}

// FromContext returns the address stored by Middleware, or "" outside a request.
func FromContext(ctx context.Context) string {
	ip, _ := ctx.Value(ctxKey{}).(string)
	return ip
}
