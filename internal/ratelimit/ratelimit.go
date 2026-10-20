// Package ratelimit is the in-binary request limiter (ADR 0025). Flood limits are
// in-memory sliding windows (go-chi/httprate); a limit of 0 disables its policy.
//
// Seams for the other slices:
//   - NewPolicy builds one named limiter; Policy.Allow checks a key and, on a
//     rejection, answers the 429 itself (headers, problem body, metric, log);
//   - Rejected is the policy-named metric + warn log for callers that render the
//     429 their own way (e.g. a typed error from an ogen security handler);
//   - WriteProblem renders the 429 body;
//   - Exchange is the request's writer and request, carried in the context by
//     Middleware, for code that only receives a ctx (the ogen security handlers):
//     it charges the Workspace budget and the failed-authentication limit and
//     reports a *LimitedError that the error handler renders with WriteProblem.
package ratelimit

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/clientip"
	"github.com/getsphericon/sphericon/internal/logging"
	"github.com/getsphericon/sphericon/internal/paths"
	"github.com/go-chi/httprate"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Policy names. They are the `policy` label of ratelimit_rejected_total, so they
// must stay a small bounded set.
const (
	PolicyHuman = "human"
	// PolicyTestSend is the cap on Test sends from the System sender (ADR 0041).
	PolicyTestSend   = "test-send"
	PolicyAPIBurst   = "api-burst"
	PolicyAPI        = "api"
	PolicyFailedAuth = "failed-auth"
	// PolicyTracking is the recording guard of opens and clicks. It never refuses.
	PolicyTracking = "tracking"
	// PolicyLoginAccount is the per-account login delay, PolicyLoginIP the per-IP cap
	// on login requests.
	PolicyLoginAccount = "login-account"
	PolicyLoginIP      = "login-ip"
	// PolicyCollect is the /collect budget per Workspace, PolicyCollectIP the one per
	// client IP.
	PolicyCollect   = "collect"
	PolicyCollectIP = "collect-ip"
	// PolicyForgotIP is the per-IP cap on forgot-password requests.
	PolicyForgotIP = "forgot-password-ip"
	// PolicyService is the budget of the internal listener's service endpoints per
	// Service credential.
	PolicyService = "service"
)

const (
	window      = time.Minute
	burstWindow = time.Second
)

// Policy is one named limiter. The zero value and nil allow everything, which is
// what a disabled (0) limit builds.
type Policy struct {
	name   string
	limit  int
	window time.Duration
	rl     *httprate.RateLimiter
}

// LimitedError is a rejection by a Policy. The 429 headers are already on the
// response writer and the rejection is counted and logged; whoever receives it
// only has to render the body with WriteProblem.
type LimitedError struct{ Policy string }

func (e *LimitedError) Error() string { return "rate limit exceeded: " + e.Policy }

// NewPolicy builds a limiter of limit requests per window. It returns nil when
// limit is not positive (disabled).
func NewPolicy(name string, limit int, window time.Duration) *Policy {
	if limit <= 0 {
		return nil
	}
	return &Policy{name: name, limit: limit, window: window, rl: httprate.NewRateLimiter(limit, window)}
}

// Allow counts one request against key and reports whether it may proceed. On a
// rejection it has already answered 429 (X-RateLimit-* and Retry-After headers,
// problem body), counted and logged it, and the caller must stop.
func (p *Policy) Allow(w http.ResponseWriter, r *http.Request, key string) bool {
	if p.Reject(w, r, key) == nil {
		return true
	}
	WriteProblem(w)
	return false
}

// Reject counts one request against key like Allow but leaves the response to the
// caller: it returns a *LimitedError (headers set, rejection counted and logged)
// when the budget is spent and nil otherwise. X-RateLimit-* headers are set on w
// either way, so they reach the successful response too.
func (p *Policy) Reject(w http.ResponseWriter, r *http.Request, key string) error {
	if p == nil || !p.rl.OnLimit(w, r, key) {
		return nil
	}
	Rejected(r.Context(), p.name)
	return &LimitedError{Policy: p.name}
}

// Blocked reports, without counting, whether key has already spent its budget. It
// answers like Reject (429 headers, counted, logged), for limits that count only
// some outcomes (failed authentications) and must refuse before the work is done.
func (p *Policy) Blocked(w http.ResponseWriter, r *http.Request, key string) error {
	if p == nil {
		return nil
	}
	ok, rate, err := p.rl.Status(key)
	if err == nil && ok && rate < float64(p.limit) {
		return nil
	}
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(p.limit))
	w.Header().Set("X-RateLimit-Remaining", "0")
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(p.window).Unix(), 10))
	w.Header().Set("Retry-After", strconv.Itoa(int(p.window.Seconds())))
	Rejected(r.Context(), p.name)
	return &LimitedError{Policy: p.name}
}

// Exceeded counts one request against key and reports whether the budget is spent.
// Unlike Allow it never answers the request: it is for a recording guard, where the
// caller keeps serving the response and only skips the side effect (tracking never
// refuses a recipient, ADR 0025). An exceeded request is counted and logged.
func (p *Policy) Exceeded(r *http.Request, key string) bool {
	if p == nil || !p.rl.OnLimit(discardWriter{}, r, key) {
		return false
	}
	Rejected(r.Context(), p.name)
	return true
}

// discardWriter swallows the X-RateLimit-* headers a recording guard must not send.
type discardWriter struct{}

func (discardWriter) Header() http.Header         { return http.Header{} }
func (discardWriter) Write(b []byte) (int, error) { return len(b), nil }
func (discardWriter) WriteHeader(int)             {}

// Rejected records one rejection under policy: ratelimit_rejected_total{policy} and
// a warn log. It carries the policy and the client address only, never an email or
// a path (paths hold tokens).
func Rejected(ctx context.Context, policy string) {
	counter, err := otel.Meter("sphericon/ratelimit").Int64Counter("ratelimit_rejected",
		metric.WithDescription("Requests refused by a rate limit, by policy."))
	if err == nil {
		counter.Add(ctx, 1, metric.WithAttributes(attribute.String("policy", policy)))
	}
	logging.FromContext(ctx).Warn("rate limit exceeded", "policy", policy, "ip", clientip.FromContext(ctx))
}

// WriteProblem renders the RFC 7807 429 body. Headers already on w (Retry-After,
// X-RateLimit-*, CORS) are kept; the Retry-After seconds are repeated in the body
// as retryAfter, so a client that only sees the body can show the wait.
func WriteProblem(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusTooManyRequests)
	body := map[string]any{
		"status": http.StatusTooManyRequests,
		"title":  http.StatusText(http.StatusTooManyRequests),
		"detail": "rate limit exceeded, retry later",
	}
	if secs, err := strconv.Atoi(w.Header().Get("Retry-After")); err == nil {
		body["retryAfter"] = secs
	}
	_ = json.NewEncoder(w).Encode(body)
}

// Limiter holds every policy built from the configured limits.
type Limiter struct {
	human      *Policy
	apiBurst   *Policy
	api        *Policy
	failedAuth *Policy
	tracking   *Policy
	loginIP    *Policy
	collect    *Policy
	collectIP  *Policy
	service    *Policy
	forgotIP   *Policy
}

// New builds the policies from limits.
func New(limits config.RateLimits) *Limiter {
	return &Limiter{
		human:      NewPolicy(PolicyHuman, limits.Human, window),
		apiBurst:   NewPolicy(PolicyAPIBurst, limits.APIBurst, burstWindow),
		api:        NewPolicy(PolicyAPI, limits.APIPerMinute, window),
		failedAuth: NewPolicy(PolicyFailedAuth, limits.FailedAuth, window),
		tracking:   NewPolicy(PolicyTracking, limits.Tracking, window),
		loginIP:    NewPolicy(PolicyLoginIP, limits.LoginIP, window),
		collect:    NewPolicy(PolicyCollect, limits.Collect, window),
		collectIP:  NewPolicy(PolicyCollectIP, limits.CollectIP, window),
		service:    NewPolicy(PolicyService, limits.ServicePerMinute, window),
		forgotIP:   NewPolicy(PolicyForgotIP, limits.ForgotIP, time.Hour),
	}
}

// Exchange is one HTTP request's writer and request, carried in the context by
// Middleware so that code which receives only a ctx (the ogen security handlers,
// where the Workspace first becomes known) can apply the limiter. Its methods are
// safe on a nil Exchange (a ctx without Middleware), where they allow everything.
type Exchange struct {
	w       http.ResponseWriter
	r       *http.Request
	limiter *Limiter
	charged atomic.Bool
}

type exchangeKey struct{}

// FromContext returns the request's Exchange, nil outside Middleware.
func FromContext(ctx context.Context) *Exchange {
	e, _ := ctx.Value(exchangeKey{}).(*Exchange)
	return e
}

// ChargeWorkspace counts the request against the Workspace budget, the burst and
// steady windows stacked. It charges at most once per HTTP request however many
// times the request is authenticated (/mcp authenticates its call again when it
// dispatches in-process to /api), so /api and /mcp draw one budget at one request
// each. Its X-RateLimit-* headers land on the shared writer and so on the
// successful response.
func (e *Exchange) ChargeWorkspace(workspaceID int64) error {
	if e == nil || !e.charged.CompareAndSwap(false, true) {
		return nil
	}
	key := strconv.FormatInt(workspaceID, 10)
	if err := e.limiter.apiBurst.Reject(e.w, e.r, key); err != nil {
		return err
	}
	return e.limiter.api.Reject(e.w, e.r, key)
}

// ChargeCollect counts the request against the Workspace's /collect budget, a
// budget of its own apart from /api and /mcp. Like ChargeWorkspace it charges at
// most once per HTTP request.
func (e *Exchange) ChargeCollect(workspaceID int64) error {
	if e == nil || !e.charged.CompareAndSwap(false, true) {
		return nil
	}
	return e.limiter.collect.Reject(e.w, e.r, strconv.FormatInt(workspaceID, 10))
}

// ChargeService counts the request against the Service credential's budget. Like the
// other charges it counts at most once per HTTP request.
func (e *Exchange) ChargeService(credentialID int64) error {
	if e == nil || !e.charged.CompareAndSwap(false, true) {
		return nil
	}
	return e.limiter.service.Reject(e.w, e.r, strconv.FormatInt(credentialID, 10))
}

// AuthBlocked refuses before a credential is even checked when the client address
// has already spent its failed-authentication budget, so guessing cannot continue
// past the limit.
func (e *Exchange) AuthBlocked(ctx context.Context) error {
	if e == nil {
		return nil
	}
	return e.limiter.failedAuth.Blocked(e.w, e.r, ipOf(ctx))
}

// AuthFailed counts one failed authentication against the client address. Only
// failures are counted, so legitimate traffic never spends this budget.
func (e *Exchange) AuthFailed(ctx context.Context) error {
	if e == nil {
		return nil
	}
	return e.limiter.failedAuth.Reject(e.w, e.r, ipOf(ctx))
}

// Refusal is a request a budget turned away: how long until a slot frees and the size of
// the budget that was spent. The zero Refusal means nothing was refused.
type Refusal struct {
	Wait  time.Duration
	Limit int
}

// Delay refuses a request that must wait out a delay rather than a window budget
// (the per-account login delay, ADR 0025): the caller may retry `wait` from now.
// limit is the budget reported in X-RateLimit-Limit; Remaining is always 0. Like
// the other rejections it sets the 429 headers on the shared writer, counts and logs
// the rejection under policy, and returns the *LimitedError the error handler
// renders.
func (e *Exchange) Delay(ctx context.Context, policy string, limit int, wait time.Duration, now time.Time) error {
	if e != nil {
		h := e.w.Header()
		h.Set("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
		h.Set("X-RateLimit-Limit", strconv.Itoa(limit))
		h.Set("X-RateLimit-Remaining", "0")
		h.Set("X-RateLimit-Reset", strconv.FormatInt(now.Add(wait).Unix(), 10))
	}
	Rejected(ctx, policy)
	return &LimitedError{Policy: policy}
}

func ipOf(ctx context.Context) string {
	return httprate.CanonicalizeIP(clientip.FromContext(ctx))
}

// RecordsTracking reports whether the engagement of this request may be recorded:
// false once the client IP spent the tracking guard. The caller must still serve the
// pixel or the redirect; only the recording is skipped.
func (l *Limiter) RecordsTracking(r *http.Request) bool {
	return !l.tracking.Exceeded(r, httprate.CanonicalizeIP(clientip.FromContext(r.Context())))
}

// Middleware applies the policy of the request's route. It sits after the client
// address middleware (the key is clientip, never a raw header) and before timeout.
// Operational endpoints are never limited, whatever policies exist.
func (l *Limiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), exchangeKey{}, &Exchange{w: w, r: r, limiter: l})
		r = r.WithContext(ctx)
		if r.Method == http.MethodPost && r.URL.Path == "/site/auth/forgot-password" &&
			!l.forgotIP.Allow(w, r, httprate.CanonicalizeIP(clientip.FromContext(ctx))) {
			return
		}
		// The per-IP login cap spans accounts (password spraying) and both login
		// steps; the per-account delay follows in the operations, which know the address.
		if r.Method == http.MethodPost && loginPath(r.URL.Path) &&
			!l.loginIP.Allow(w, r, "login|"+httprate.CanonicalizeIP(clientip.FromContext(ctx))) {
			return
		}
		if !exempt(r.URL.Path) {
			// The per-IP /collect budget needs no credential, so it is applied up front
			// (CORS has already answered preflights); the Workspace budget follows the
			// collect key in the security handler.
			if strings.HasPrefix(r.URL.Path, "/collect/") &&
				!l.collectIP.Allow(w, r, httprate.CanonicalizeIP(clientip.FromContext(ctx))) {
				return
			}
			if route := humanRoute(r); route != "" &&
				!l.human.Allow(w, r, httprate.CanonicalizeIP(clientip.FromContext(ctx))+"|"+route) {
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// loginPath is the two steps of a login on either surface, /site and /staff.
func loginPath(path string) bool {
	switch path {
	case "/site/auth/login", "/site/auth/second-factor", paths.Staff + "/auth/login", paths.Staff + "/auth/second-factor":
		return true
	}
	return false
}

// exempt lists what is never limited: probes, metrics and the provider hooks
// (authenticated by their ingest key).
func exempt(path string) bool {
	return path == "/healthz" || path == "/readyz" || path == "/metrics" || strings.HasPrefix(path, "/hooks/")
}

// humanRoute names the public human-facing endpoints (one bucket per IP and
// route): signup, resend-verification, verify-email, invitation accept and consent confirm. "" means not one.
func humanRoute(r *http.Request) string {
	if r.Method != http.MethodPost {
		return ""
	}
	p := r.URL.Path
	switch {
	case p == "/site/auth/register":
		return "signup"
	case p == "/site/auth/resend-verification":
		return "resend-verification"
	case p == "/site/auth/verify-email":
		return "verify-email"
	case strings.HasPrefix(p, "/site/invitations/") && strings.HasSuffix(p, "/accept"):
		return "invitation-accept"
	case strings.HasPrefix(p, "/site/confirmations/") && !strings.Contains(strings.TrimPrefix(p, "/site/confirmations/"), "/"):
		return "consent-confirm"
	}
	return ""
}
