// Package clientip carries the best-effort client address of a request through the
// context, so ogen handlers (which never see the *http.Request) can record it as
// proof, e.g. on a double-opt-in confirmation.
package clientip

import (
	"context"
	"net"
	"net/http"

	"github.com/realclientip/realclientip-go"
)

type (
	ctxKey          struct{}
	userAgentCtxKey struct{}
)

// forwardedFor reads exactly one trusted proxy hop: the binary runs behind one
// Caddy/ingress, which appends the connecting address to X-Forwarded-For. Taking the
// rightmost entry means a client cannot spoof its address (and with it the recorded
// consent proof) by sending its own X-Forwarded-For. Raise the count if more proxies
// sit in front.
var forwardedFor = func() realclientip.Strategy {
	strategy, err := realclientip.NewRightmostTrustedCountStrategy("X-Forwarded-For", 1)
	if err != nil {
		panic(err) // static header name and count: cannot fail
	}
	return strategy
}()

// FromRequest returns the address our trusted proxy saw, falling back to the
// connection's remote host when the request did not come through one.
func FromRequest(r *http.Request) string {
	if ip := forwardedFor.ClientIP(r.Header, r.RemoteAddr); ip != "" {
		return ip
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// Middleware stores FromRequest and the User-Agent in the request context for
// FromContext and UserAgentFromContext.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), ctxKey{}, FromRequest(r))
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
