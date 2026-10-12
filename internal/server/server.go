package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent"
	collectapi "github.com/getsphericon/sphericon/gen/collect"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	operatorapi "github.com/getsphericon/sphericon/gen/operator"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accountlink"
	apiauth "github.com/getsphericon/sphericon/internal/api/auth"
	apicollect "github.com/getsphericon/sphericon/internal/api/collect"
	apiexternal "github.com/getsphericon/sphericon/internal/api/external"
	apisite "github.com/getsphericon/sphericon/internal/api/site"
	"github.com/getsphericon/sphericon/internal/clientip"
	"github.com/getsphericon/sphericon/internal/logging"
	"github.com/getsphericon/sphericon/internal/oauthserver"
	"github.com/getsphericon/sphericon/internal/paths"
	"github.com/getsphericon/sphericon/internal/ratelimit"
	"github.com/ogen-go/ogen/ogenerrors"
	"github.com/oklog/ulid/v2"
	"github.com/rs/cors"
)

// New builds the top-level net/http handler wiring the three ogen-generated
// API servers (site, external, collect) and the public endpoints.
// New composes the HTTP handler. client is the raw ent client: only the pieces whose
// Workspace is not known up front take it (auth, OAuth, tracking, provider hooks);
// the site, external and collect handlers get none (ADR 0017).
func New(cfg *config.Config, db *sql.DB, client *ent.Client, site apisite.Deps, external, mcp, operator http.Handler) (http.Handler, error) {
	bus := site.Bus
	mux := http.NewServeMux()

	// Login and logout are /site operations (ADR 0020), and the login operation is
	// the one route that mints a session, through this issuer. The library's
	// /auth/ and /avatar/ routes are gone and must not fall through to the SPA shell.
	site.Sessions = apiauth.NewSessions(cfg.JWTSecret, cfg.SessionTTL, cfg.SecureCookies(), site.Clock)
	mux.Handle("/auth/", http.NotFoundHandler())
	mux.Handle("/avatar/", http.NotFoundHandler())
	limiter := ratelimit.New(cfg.RateLimits)

	// Site API — /site (JWT cookie via generated SecurityHandler; register, login
	// and logout are public per the spec).
	siteSrv, err := siteapi.NewServer(
		apisite.NewHandlers(site),
		apiauth.NewSiteSecurityHandler(cfg.JWTSecret, client, site.Clock),
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

	// Operator API — /operator (ADR 0026): the platform staff surface, its own cookie and
	// secret. Without the `operator` license there is no handler and the whole prefix
	// answers 404, so it never falls through to the SPA shell below.
	if operator == nil {
		operator = http.NotFoundHandler()
	}
	mux.Handle(operatorPrefix+"/", operator)

	// The service endpoints of the Operator surface are served only by the internal
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
	oauthserver.New(client, cfg.AppURL, cfg.MCPOrigin()).Mount(mux)

	// Collect API — /collect (x-collect-key via generated SecurityHandler).
	colSrv, err := collectapi.NewServer(
		apicollect.NewHandlers(bus),
		apiauth.NewCollectSecurityHandler(client),
		collectapi.WithPathPrefix(collectPrefix),
		collectapi.WithErrorHandler(problemErrorHandler),
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
	mux.Handle("/e/", trackingHandler(client, bus, site.Tracker, limiter))

	// Inbound provider webhooks (SES bounce/complaint via SNS), routed by the
	// workspace's secret ingest key: POST /hooks/{key}/{provider}.
	mux.Handle("/hooks/", hooksHandler(client, bus))

	// Catch-all: the embedded SPA (release builds with -tags embed_spa). Most
	// specific pattern wins, so this never shadows the API prefixes above.
	mux.Handle("/", spaHandler(cfg.Locale))

	guard, err := crossOriginGuard(cfg.AppURL, cfg.CORSOrigins)
	if err != nil {
		return nil, err
	}

	// Order (ADR 0025): recoverer, requestID, CORS, client address, rate limit,
	// timeout. CORS precedes the limiter so a 429 still reaches the browser; guard
	// sits inside CORS so preflights are answered before the check.
	return chain(mux, recoverer, requestID, corsMiddleware(cfg.CORSOrigins), clientip.Middleware, limiter.Middleware, timeout(30*time.Second), bodyLimit(cfg.BodyLimits), collectEventLimit(cfg.BodyLimits.CollectEvent), guard), nil
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

// operatorPrefix mounts the Operator surface (ADR 0026).
const operatorPrefix = paths.Operator

// OperatorSurface is the Enterprise Operator API (ee/operator), built into an ogen
// server here so it renders errors like the other surfaces.
type OperatorSurface interface {
	Server(opts ...operatorapi.ServerOption) (http.Handler, error)
}

// NewOperatorAPI builds the /operator ogen server with RFC 7807 errors. A nil surface
// (no `operator` license) yields a nil handler, which New mounts as a 404.
func NewOperatorAPI(surface OperatorSurface) (http.Handler, error) {
	if surface == nil {
		return nil, nil
	}
	return surface.Server(operatorapi.WithErrorHandler(problemErrorHandler))
}

// ServiceSurface is the service endpoints of the Operator surface (ee/operator), built
// into an ogen server here so it renders errors like the other surfaces.
type ServiceSurface interface {
	Server(opts ...serviceapi.ServerOption) (http.Handler, error)
}

// NewInternal builds the handler of the internal listener (ADR 0026): the service
// endpoints under /service and nothing else, behind the middleware the public server
// uses where it applies (recoverer, request id, client address, rate limit, timeout,
// body limit). A nil surface (no `operator` license) yields a nil handler, and the
// composition root then binds no listener at all.
func NewInternal(cfg *config.Config, surface ServiceSurface) (http.Handler, error) {
	if surface == nil {
		return nil, nil
	}
	srv, err := surface.Server(serviceapi.WithErrorHandler(problemErrorHandler))
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.Handle(paths.Service+"/", srv)
	limiter := ratelimit.New(cfg.RateLimits)
	return chain(mux, recoverer, requestID, clientip.Middleware, limiter.Middleware, timeout(30*time.Second), bodyLimit(cfg.BodyLimits)), nil
}

// problemErrorHandler renders ogen errors as RFC 7807 application/problem+json.
// A reference id from another Workspace (the scoped client's ErrNotInWorkspace,
// ADR 0017) is a client error, so it is a 422 and never a 500.
func problemErrorHandler(_ context.Context, w http.ResponseWriter, _ *http.Request, err error) {
	// A rate limit rejection already set its headers on the shared writer and was
	// counted and logged where it was decided (ratelimit.LimitedError).
	var limited *ratelimit.LimitedError
	if errors.As(err, &limited) {
		ratelimit.WriteProblem(w)
		return
	}
	// A handler error that knows its own problem (the site's
	// second_factor_required, ADR 0020) is rendered as it says.
	var known problemError
	if errors.As(err, &known) {
		status, code, detail := known.Problem()
		writeCodedProblem(w, status, code, detail)
		return
	}
	code := http.StatusInternalServerError
	var oe ogenerrors.Error
	var tooBig *http.MaxBytesError
	switch {
	case errors.As(err, &tooBig):
		code = http.StatusRequestEntityTooLarge
	case errors.As(err, &oe):
		code = oe.Code()
	case errors.Is(err, ent.ErrNotInWorkspace):
		code = http.StatusUnprocessableEntity
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(code)
	prob := map[string]any{
		"status": code,
		"title":  http.StatusText(code),
	}
	if code < http.StatusInternalServerError {
		prob["detail"] = err.Error()
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
	_ = json.NewEncoder(w).Encode(prob)
}

// --- cross-cutting net/http middleware ---

func chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
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
				logger.Error("panic recovered",
					"err", rec,
					"method", r.Method,
					"path", r.URL.Path,
					"stack", string(debug.Stack()),
				)
				writeProblem(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// timeout bounds every request's context (hang protection; replaces echo ContextTimeout).
func timeout(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
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
// API and the Operator API), the only surfaces exposed to CSRF.
func cookiePath(path string) bool {
	return strings.HasPrefix(path, "/site/") || strings.HasPrefix(path, operatorPrefix+"/")
}

// corsMiddleware applies three rs/cors policies by path:
//   - /collect/: echoes any origin without credentials (the collect key is
//     public, cookies are first-party on the customer's own domain);
//   - /site/ and /operator/ (cookie auth): credentials only for the configured
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
// The app origin (APP_URL, which may carry a path or trailing slash) and the
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
