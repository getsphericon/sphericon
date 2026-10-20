package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent"
	collectapi "github.com/getsphericon/sphericon/gen/collect"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	staffapi "github.com/getsphericon/sphericon/gen/staff"
	"github.com/getsphericon/sphericon/internal/accountlink"
	apiauth "github.com/getsphericon/sphericon/internal/api/auth"
	apicollect "github.com/getsphericon/sphericon/internal/api/collect"
	apiexternal "github.com/getsphericon/sphericon/internal/api/external"
	apisite "github.com/getsphericon/sphericon/internal/api/site"
	"github.com/getsphericon/sphericon/internal/clientip"
	dbpkg "github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/getsphericon/sphericon/internal/lifecycle"
	"github.com/getsphericon/sphericon/internal/logging"
	"github.com/getsphericon/sphericon/internal/oauthserver"
	"github.com/getsphericon/sphericon/internal/paths"
	"github.com/getsphericon/sphericon/internal/ratelimit"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/ogen-go/ogen/ogenerrors"
	"github.com/oklog/ulid/v2"
	"github.com/rs/cors"
)

// New builds the top-level net/http handler wiring the three ogen-generated
// API servers (site, external, collect) and the public endpoints.
// New composes the HTTP handler. client is the raw ent client: only the pieces whose
// Workspace is not known up front take it (auth, OAuth, tracking, provider hooks);
// the site, external and collect handlers get none (ADR 0017).
func New(cfg *config.Config, db *sql.DB, client *ent.Client, site apisite.Deps, external, mcp, staff http.Handler) (http.Handler, error) {
	bus := site.Bus
	mux := http.NewServeMux()

	// Login and logout are /site operations (ADR 0020), and the login operation is
	// the one route that mints a session, through this issuer. The library's
	// /auth/ and /avatar/ routes are gone and must not fall through to the SPA shell.
	site.Sessions = apiauth.NewSessions(cfg.JWTSecret, cfg.SessionTTL, cfg.SecureCookies(), site.Clock, cfg.JWTVerifySecrets()...)
	mux.Handle("/auth/", http.NotFoundHandler())
	mux.Handle("/avatar/", http.NotFoundHandler())
	limiter := ratelimit.New(cfg.RateLimits)

	// Site API — /site (JWT cookie via generated SecurityHandler; register, login
	// and logout are public per the spec).
	siteSrv, err := siteapi.NewServer(
		apisite.NewHandlers(site),
		apiauth.NewSiteSecurityHandler(cfg.JWTSecret, client, site.Clock, cfg.JWTVerifySecrets()...),
		siteapi.WithPathPrefix("/site"),
		siteapi.WithErrorHandler(problemErrorHandler),
	)
	if err != nil {
		return nil, err
	}
	mux.Handle("/site/", siteSrv)

	// The Account link's public key set: published only when the link is configured.
	if site.AccountLink != nil {
		mux.Handle("GET "+accountlink.JWKSPath, jwksHandler(site.AccountLink.JWKS()))
	}

	// Staff member API — /staff (ADR 0026): the platform staff surface, its own cookie and
	// secret. Without the `staff` license there is no handler and the whole prefix
	// answers 404, so it never falls through to the SPA shell below.
	if staff == nil {
		staff = http.NotFoundHandler()
	}
	mux.Handle(staffPrefix+"/", staff)

	// The service endpoints of the Staff surface are served only by the internal
	// listener (ADR 0026). The public port has no route for them: the prefix is a 404
	// here, so it never falls through to the SPA shell below either.
	mux.Handle(paths.Service+"/", http.NotFoundHandler())

	// External API — /api (Bearer token auth via ogen SecurityHandler); built by
	// NewExternalAPI so the MCP surface dispatches through the same server.
	mux.Handle("/api/", external)

	// MCP — /mcp (ADR 0016): tools projected from the external contract and
	// dispatched in-process through the handler above with the caller's own token.
	mux.Handle("/mcp", mcp)

	// OAuth 2.1 for MCP connectors (ADR 0016): discovery, dynamic client
	// registration, authorize (hands off to the SPA consent screen) and token.
	oauthserver.New(site.OAuth, cfg.AppURL, cfg.MCPOrigin()).Mount(mux)

	// Collect API — /collect (x-collect-key via generated SecurityHandler).
	colSrv, err := collectapi.NewServer(
		apicollect.NewHandlers(bus),
		apiauth.NewCollectSecurityHandler(client),
		collectapi.WithPathPrefix(collectPrefix),
		collectapi.WithErrorHandler(collectErrorHandler),
	)
	if err != nil {
		return nil, err
	}
	mux.Handle(collectPrefix+"/", colSrv)

	// Liveness/readiness probes (no auth) for orchestrators and load balancers.
	mux.Handle("/healthz", healthzHandler())
	mux.Handle("/readyz", readyzHandler(db))

	// Public tracking snippet (no auth) — embedded IIFE bundle.
	mux.Handle("/t.js", trackerHandler())

	// Public email engagement endpoints (open pixel, click redirect, unsubscribe).
	trackingH := trackingHandler(client, bus, site.Tracker, limiter, cfg.AppURL)
	mux.Handle("/e/", trackingH)

	// Inbound provider callbacks (ADR 0037): POST /hooks/{key}, the key of one
	// Integration, read with that Integration's Preset.
	mux.Handle("/hooks/", hooksHandler(client, bus, site.Integrations, site.Clock))

	// Catch-all: the embedded SPA (release builds with -tags embed_spa). Most
	// specific pattern wins, so this never shadows the API prefixes above.
	mux.Handle("/", spaHandler(cfg.Locale, cfg.RequireEmailVerification))

	guard, err := crossOriginGuard(cfg.AppURL, cfg.CORSOrigins)
	if err != nil {
		return nil, err
	}

	// Order (ADR 0025): recoverer, requestID, CORS, client address, rate limit,
	// timeout. CORS precedes the limiter so a 429 still reaches the browser; guard
	// sits inside CORS so preflights are answered before the check.
	outer := chi.Middlewares{recoverer, requestID, corsMiddleware(cfg.CORSOrigins)}
	if cfg.IsDev {
		// Development only: a 500 carries what failed, for the page to show (devdebug.go).
		outer = append(chi.Middlewares{devDebug}, outer...)
	}
	if cfg.IsDev && !cfg.AutoMigrate {
		// Development only (ADR 0033): a database reset or rebuilt under the running process
		// is named instead of answering bare 500s. A production Instance never answers 503 on
		// a check of its own; the start guard is its whole protection.
		outer = append(outer, SchemaGuard(dbpkg.NewSchemaWatch(db)))
	}
	inner := chi.Middlewares{clientip.Middleware, limiter.Middleware, middleware.Timeout(30 * time.Second), bodyLimit(cfg.BodyLimits), noNUL, collectEventLimit(cfg.BodyLimits.CollectEvent), guard}
	return chi.Chain(append(outer, inner...)...).Handler(trackingHostGate(cfg, client, mux, trackingH)), nil
}

// NewExternalAPI builds the external API (/api) ogen server: Bearer API-token
// auth, RFC 7807 errors, mounted under the /api prefix.
func NewExternalAPI(client *ent.Client, deps apiexternal.Deps) (http.Handler, error) {
	return externalapi.NewServer(
		apiexternal.NewHandlers(deps),
		apiauth.NewExternalSecurityHandler(client, deps.Bus),
		externalapi.WithPathPrefix("/api"),
		externalapi.WithErrorHandler(problemErrorHandler),
	)
}

// staffPrefix mounts the Staff surface (ADR 0026).
const staffPrefix = paths.Staff

// StaffSurface is the Enterprise Staff member API (ee/staff), built into an ogen
// server here so it renders errors like the other surfaces.
type StaffSurface interface {
	Server(opts ...staffapi.ServerOption) (http.Handler, error)
}

// NewStaffAPI builds the /staff ogen server with RFC 7807 errors. A nil surface
// (no `staff` license) yields a nil handler, which New mounts as a 404.
func NewStaffAPI(surface StaffSurface) (http.Handler, error) {
	if surface == nil {
		return nil, nil
	}
	return surface.Server(staffapi.WithErrorHandler(problemErrorHandler))
}

// ServiceSurface is the service endpoints of the Staff surface (ee/staff), built
// into an ogen server here so it renders errors like the other surfaces.
type ServiceSurface interface {
	Server(opts ...serviceapi.ServerOption) (http.Handler, error)
}

// NewInternal builds the handler of the internal listener (ADR 0026): the service
// endpoints under /service and nothing else, behind the middleware the public server
// uses where it applies (recoverer, request id, client address, rate limit, timeout,
// body limit). The edge's on-demand TLS question (paths.TLSAsk, ADR 0038) is core and
// always served; the service endpoints exist only with a surface, so a nil one (no
// `staff` license) leaves /service unrouted (404).
func NewInternal(cfg *config.Config, client *ent.Client, surface ServiceSurface) (http.Handler, error) {
	mux := http.NewServeMux()
	mux.Handle("GET "+paths.TLSAsk, tlsAskHandler(client))
	if surface != nil {
		srv, err := surface.Server(serviceapi.WithErrorHandler(problemErrorHandler))
		if err != nil {
			return nil, err
		}
		mux.Handle(paths.Service+"/", srv)
	}
	limiter := ratelimit.New(cfg.RateLimits)
	return chi.Chain(recoverer, requestID, clientip.Middleware, limiter.Middleware, middleware.Timeout(30*time.Second), bodyLimit(cfg.BodyLimits), noNUL).Handler(mux), nil
}

// collectRetryAfter is the Retry-After (seconds) of a /collect 503: long enough for a
// restarting database to come back, short enough that a client's events stay fresh.
const collectRetryAfter = "5"

// renderRetryAfter is the Retry-After (seconds) of a 503 for a document render that could not run.
const renderRetryAfter = "5"

// collectErrorHandler is problemErrorHandler for /collect, which also tells its client
// to retry: a database that cannot be reached is a 503 with Retry-After, because a
// retried event is counted once (clientEventId) and a client can send it again safely.
func collectErrorHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, err error) {
	if dbpkg.IsUnavailable(err) {
		w.Header().Set("Retry-After", collectRetryAfter)
		writeProblem(w, http.StatusServiceUnavailable, "database unavailable, retry later")
		return
	}
	problemErrorHandler(ctx, w, r, err)
}

// problemErrorHandler renders ogen errors as RFC 7807 application/problem+json.
// A reference id from another Workspace (the scoped client's ErrNotInWorkspace,
// ADR 0017) and a value an ent validator refuses are client errors, so they are a 422 and never
// a 500.
func problemErrorHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, err error) {
	// A rate limit rejection already set its headers on the shared writer and was
	// counted and logged where it was decided (ratelimit.LimitedError).
	if _, ok := errors.AsType[*ratelimit.LimitedError](err); ok {
		ratelimit.WriteProblem(w)
		return
	}
	// A handler error that knows its own problem (the site's
	// second_factor_required, ADR 0020) is rendered as it says.
	if known, ok := errors.AsType[problemError](err); ok {
		status, code, detail := known.Problem()
		writeCodedProblem(w, status, code, detail)
		return
	}
	// A render that could not run (the request ended, a limit of ours stopped it) is no fault of
	// the document and no crash: the client may try again.
	if errors.Is(err, emailrender.ErrRenderUnavailable) {
		w.Header().Set("Retry-After", renderRetryAfter)
		writeProblem(w, http.StatusServiceUnavailable, emailrender.ErrRenderUnavailable.Error())
		return
	}
	code := http.StatusInternalServerError
	var oe ogenerrors.Error
	var tooBig *http.MaxBytesError
	var invalid *ent.ValidationError
	switch {
	case errors.As(err, &tooBig):
		code = http.StatusRequestEntityTooLarge
	case errors.As(err, &oe):
		code = oe.Code()
	case errors.Is(err, lifecycle.ErrInvalidTransition):
		// A move the entity's lifecycle refuses, or lost to a concurrent writer (ADR 0046).
		code = http.StatusConflict
	case errors.Is(err, ent.ErrNotInWorkspace):
		code = http.StatusUnprocessableEntity
	case errors.As(err, &invalid):
		// A value the schema's validators refuse (an empty name, a too long label) is the
		// client's to fix, never a server fault.
		code = http.StatusUnprocessableEntity
	}
	// The client is told nothing about a server fault, so the log is the only place it shows.
	if code >= http.StatusInternalServerError {
		slog.ErrorContext(ctx, "request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(code)
	prob := map[string]any{
		"status": code,
		"title":  http.StatusText(code),
	}
	if code < http.StatusInternalServerError {
		prob["detail"] = err.Error()
	} else if dbg := debugProblem(r, w, err.Error(), ""); dbg != nil {
		prob["debug"] = dbg
	}
	_ = json.NewEncoder(w).Encode(prob)
}

// problemError is an error a handler returns that carries the problem it is
// answered with: the HTTP status, the machine-readable problem code and the
// localized detail.
type problemError interface {
	error
	Problem() (status int, code, detail string)
}

func writeProblem(w http.ResponseWriter, status int, detail string) {
	writeCodedProblem(w, status, "", detail)
}

// writeCodedProblem renders an RFC 7807 problem; code, when set, is the problem's
// machine-readable code.
func writeCodedProblem(w http.ResponseWriter, status int, code, detail string) {
	writeCodedProblemWith(w, status, code, detail, nil)
}

// writeCodedProblemWith is writeCodedProblem plus the debug block a development server adds
// to a 500 (nil: none).
func writeCodedProblemWith(w http.ResponseWriter, status int, code, detail string, dbg *problemDebug) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	prob := map[string]any{
		"status": status,
		"title":  http.StatusText(status),
		"detail": detail,
	}
	if code != "" {
		prob["code"] = code
	}
	if dbg != nil {
		prob["debug"] = dbg
	}
	_ = json.NewEncoder(w).Encode(prob)
}

// --- cross-cutting net/http middleware ---
//
// chi supplies the chain and the timeout. recoverer and requestID stay ours:
// chi's Recoverer answers a bare 500 (we render RFC 7807 and log through slog),
// and chi's RequestID generates host-counter ids from a package-level generator
// that cannot be replaced (we keep ULIDs and tag logging.FromContext).

// PendingMigrations is what SchemaGuard asks: how many migrations the database has not run.
type PendingMigrations interface {
	Pending(ctx context.Context) (int, error)
}

// SchemaGuard answers 503 with a problem that names the migrate command while the database
// lacks migrations this binary needs, instead of letting every query fail with a bare 500.
// The probe paths pass: /healthz is liveness only, and /readyz reports on the database itself.
// A failed lookup lets the request through: an unreachable database fails it on its own.
func SchemaGuard(watch PendingMigrations) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
				next.ServeHTTP(w, r)
				return
			}
			pending, err := watch.Pending(r.Context())
			if err != nil || pending == 0 {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Retry-After", "5")
			writeProblem(w, http.StatusServiceUnavailable, fmt.Sprintf(
				"The database is %d migration(s) behind this build. Run `sphericon migrate` (in development `mise run db:migrate`), then reload.",
				pending))
		})
	}
}

func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := logging.FromContext(r.Context())
		defer func() {
			if rec := recover(); rec != nil {
				// recoverer is outermost, so requestID has only put the id on the
				// shared response headers, not in this request's context.
				if id := w.Header().Get("X-Request-Id"); id != "" {
					logger = logger.With("request_id", id)
				}
				stack := string(debug.Stack())
				logger.Error("panic recovered",
					"err", rec,
					"method", r.Method,
					"path", r.URL.Path,
					"stack", stack,
				)
				dbg := debugProblem(r, w, fmt.Sprint(rec), stack)
				writeCodedProblemWith(w, http.StatusInternalServerError, "", "internal server error", dbg)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// collectPrefix mounts the public tracking ingestion; it is cut off from the rest
// of the app by its own CORS policy, auth scheme and body cap.
const collectPrefix = "/collect"

func isCollectPath(path string) bool { return strings.HasPrefix(path, collectPrefix+"/") }

// bodyLimit caps every request body before a handler or the ogen decoder reads it:
// collect gets its own (smaller) caps (a batch of events, or a single event), everything else the default. An oversized
// body surfaces as *http.MaxBytesError, which problemErrorHandler renders as 413.
func bodyLimit(limits config.BodyLimits) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			limit := limits.Default
			switch {
			case r.URL.Path == collectPrefix+"/events":
				limit = limits.Collect // a batch
			case isCollectPath(r.URL.Path):
				limit = limits.CollectEvent // one event
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}

func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" {
			id = ulid.Make().String()
		}
		w.Header().Set("X-Request-Id", id)
		// Carry the id in context so request-scoped logs (via logging.FromContext)
		// correlate back to this request.
		next.ServeHTTP(w, r.WithContext(logging.WithRequestID(r.Context(), id)))
	})
}

// cookiePath reports whether a path is authenticated by a session cookie (the SPA
// API and the Staff member API), the only surfaces exposed to CSRF.
func cookiePath(path string) bool {
	return strings.HasPrefix(path, "/site/") || strings.HasPrefix(path, staffPrefix+"/")
}

// corsMiddleware applies three rs/cors policies by path:
//   - /collect/: echoes any origin without credentials (the collect key is
//     public, cookies are first-party on the customer's own domain);
//   - /site/ and /staff/ (cookie auth): credentials only for the configured
//     allowlist. The SPA is served same-origin, so with no allowlist there are no
//     CORS headers at all;
//   - everything else (external API, MCP, OAuth: bearer tokens): echoes any origin
//     without credentials.
func corsMiddleware(origins []string) func(http.Handler) http.Handler {
	methods := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions}
	// "*" reflects whatever request headers the client asks for (works with
	// credentials, and avoids brittle exact-match of comma-joined header lists).
	anyHeaders := []string{"*"}
	open := cors.New(cors.Options{
		AllowedMethods:  methods,
		AllowedHeaders:  anyHeaders,
		AllowOriginFunc: func(string) bool { return true },
	})
	collect := cors.New(cors.Options{
		AllowedMethods:  []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		AllowedHeaders:  anyHeaders,
		AllowOriginFunc: func(string) bool { return true },
	})
	var cookie *cors.Cors
	if len(origins) > 0 {
		cookie = cors.New(cors.Options{
			AllowedMethods:   methods,
			AllowedHeaders:   anyHeaders,
			AllowCredentials: true,
			AllowedOrigins:   origins,
		})
	}

	return func(next http.Handler) http.Handler {
		openH := open.Handler(next)
		collectH := collect.Handler(next)
		cookieH := next
		if cookie != nil {
			cookieH = cookie.Handler(next)
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case isCollectPath(r.URL.Path):
				collectH.ServeHTTP(w, r)
			case cookiePath(r.URL.Path):
				cookieH.ServeHTTP(w, r)
			default:
				openH.ServeHTTP(w, r)
			}
		})
	}
}

// crossOriginGuard rejects cross-site unsafe requests (Sec-Fetch-Site, falling back
// to Origin vs Host) on the cookie-authenticated paths with the stdlib's
// http.CrossOriginProtection. This is why the session carries no XSRF
// double-submit token: it would need a token round-trip in the generated client
// for the same protection. Bearer and secret-key surfaces (collect, API, MCP, OAuth, hooks,
// tracking) are cross-origin by design and pass through untouched.
//
// The app origin (SPHERICON_APP_URL, which may carry a path or trailing slash) and the
// credentialed-CORS allowlist are trusted: an origin allowed to send cookies
// cross-origin must also be allowed to write.
func crossOriginGuard(appURL string, allowed []string) (func(http.Handler) http.Handler, error) {
	cop := http.NewCrossOriginProtection()
	for _, raw := range append([]string{appURL}, allowed...) {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return nil, fmt.Errorf("trusted origin %q: want scheme://host", raw)
		}
		if err := cop.AddTrustedOrigin(u.Scheme + "://" + u.Host); err != nil {
			return nil, fmt.Errorf("trusted origin %q: %w", raw, err)
		}
	}
	return func(next http.Handler) http.Handler {
		guarded := cop.Handler(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cookiePath(r.URL.Path) {
				guarded.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}, nil
}
