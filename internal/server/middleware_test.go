package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"entgo.io/ent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	entgen "github.com/getsphericon/sphericon/ent"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	apisite "github.com/getsphericon/sphericon/internal/api/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/server"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// serviceFake is a service surface that answers with the handler a test controls.
type serviceFake struct{ handler http.Handler }

func (f serviceFake) Server(...serviceapi.ServerOption) (http.Handler, error) { return f.handler, nil }

// internalServer is the internal listener with handler as its service surface (under /service/):
// the middleware every listener runs, around a handler the test controls. No request of these
// tests asks the TLS question, the one route that reads the database, so it has no client.
func internalServer(t *testing.T, handler http.Handler) http.Handler {
	t.Helper()
	h, err := server.NewInternal(&config.Config{BodyLimits: config.BodyLimits{Default: 1 << 20}}, nil, serviceFake{handler})
	require.NoError(t, err)
	return h
}

// statusHandler answers every request with status.
func statusHandler(status int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) })
}

func serveRequest(t *testing.T, h http.Handler, method, target string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, target, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// panicOnVisitorWrites makes the transaction collect opens panic with value.
func panicOnVisitorWrites(env *testhelper.TestEnv, value string) {
	env.Bus.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			if m.Type() == entgen.TypeVisitor {
				panic(value)
			}
			return next.Mutate(ctx, m)
		})
	})
}

// production builds the server as a production Instance runs it: no development debug block.
func production() testhelper.Option {
	return testhelper.WithConfig(func(c *config.Config) { c.IsDev = false })
}

// development builds the server as a development Instance runs it.
func development() testhelper.Option {
	return testhelper.WithConfig(func(c *config.Config) { c.IsDev = true })
}

// problemDebug is the `debug` member of a development server's 500 problem.
type problemDebug struct {
	Error  string `json:"error"`
	Stack  string `json:"stack"`
	Method string `json:"method"`
	Path   string `json:"path"`
}

func debugOf(t *testing.T, rec *httptest.ResponseRecorder) problemDebug {
	t.Helper()
	var body struct {
		Debug problemDebug `json:"debug"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), rec.Body.String())
	return body.Debug
}

func TestProbesOnlyAnswerGet(t *testing.T) {
	env := testhelper.Setup(t)

	for _, path := range []string{"/healthz", "/readyz"} {
		rec := serveRequest(t, env.Server, http.MethodPost, path, nil)

		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code, path)
		assert.Equal(t, http.MethodGet, rec.Header().Get("Allow"), path)
	}
}

// A ping that fails reports the replica unready. The request's context has ended, so the ping
// fails before it reaches the database.
func TestReadyzIsUnavailableWhenTheDatabaseCannotBePinged(t *testing.T) {
	env := testhelper.Setup(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	rec := httptest.NewRecorder()
	env.Server.ServeHTTP(rec, httptest.NewRequestWithContext(ctx, http.MethodGet, "/readyz", nil))

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	assert.Contains(t, rec.Body.String(), "database unreachable")
}

// A body that cannot be decoded is the client's to fix: a 400 that says why.
func TestAnUndecodableBodyIsA400ProblemThatExplainsItself(t *testing.T) {
	env := testhelper.Setup(t)

	rec := servePost(t, env, "/api/contacts", `{`, map[string]string{"Authorization": "Bearer " + env.ScopedBearer(t, "contacts:write")})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	assert.Contains(t, rec.Body.String(), `"title":"Bad Request"`)
	assert.Contains(t, rec.Body.String(), "decode request")
}

// Any other failure is a 500 that never leaks its cause to the client.
func TestAServerFaultIsA500ThatNeverLeaksTheCause(t *testing.T) {
	env := testhelper.Setup(t, production())
	failVisitorWrites(env, errors.New("db password is hunter2"))

	rec := postCollect(t, env, collectReq{key: fixtures.AcmeCollectKey})

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.JSONEq(t, `{"status":500,"title":"Internal Server Error"}`, rec.Body.String())
}

func TestRequestIDIsEchoedOrGenerated(t *testing.T) {
	env := testhelper.Setup(t)

	echoed := serveRequest(t, env.Server, http.MethodGet, "/healthz", map[string]string{"X-Request-ID": "abc-123"})
	first := serveRequest(t, env.Server, http.MethodGet, "/healthz", nil).Header().Get("X-Request-ID")
	second := serveRequest(t, env.Server, http.MethodGet, "/healthz", nil).Header().Get("X-Request-ID")

	assert.Equal(t, "abc-123", echoed.Header().Get("X-Request-ID"))
	assert.Len(t, first, 26, "a ULID")
	assert.NotEqual(t, first, second)
}

// preflight asks h whether origin may POST to path.
func preflight(t *testing.T, h http.Handler, path, origin string) http.Header {
	t.Helper()
	return serveRequest(t, h, http.MethodOptions, path, map[string]string{
		"Origin":                        origin,
		"Access-Control-Request-Method": "POST",
	}).Header()
}

func withCORSOrigins(origins ...string) testhelper.Option {
	return testhelper.WithConfig(func(c *config.Config) { c.CORSOrigins = origins })
}

// The collect API and the bearer-token surfaces (external API, MCP, OAuth) echo any origin but
// never allow credentials, whatever the allowlist.
func TestCollectAndBearerSurfacesEchoAnyOriginWithoutCredentials(t *testing.T) {
	for _, origins := range [][]string{nil, {"https://app.example"}} {
		env := testhelper.Setup(t, withCORSOrigins(origins...))

		for _, path := range []string{"/collect/events", "/api/contacts", "/mcp", "/oauth/token"} {
			got := preflight(t, env.Server, path, "https://customer.example")

			assert.Equal(t, "https://customer.example", got.Get("Access-Control-Allow-Origin"), path)
			assert.Empty(t, got.Get("Access-Control-Allow-Credentials"), path)
		}
	}
}

// Cookie-authenticated paths allow cross-origin credentials only for an explicit allowlist:
// with none configured there are no CORS headers at all.
func TestCookiePathsSendNoCORSHeadersWithoutAnAllowlist(t *testing.T) {
	env := testhelper.Setup(t, withCORSOrigins())

	for _, path := range []string{"/site/workspaces", "/site/auth/login"} {
		got := preflight(t, env.Server, path, "https://customer.example")

		assert.Empty(t, got.Get("Access-Control-Allow-Origin"), path)
		assert.Empty(t, got.Get("Access-Control-Allow-Credentials"), path)
	}
}

func TestCookiePathsAllowCredentialsOnlyForAnAllowlistedOrigin(t *testing.T) {
	env := testhelper.Setup(t, withCORSOrigins("https://app.example"))

	for _, path := range []string{"/site/workspaces", "/site/auth/login"} {
		other := preflight(t, env.Server, path, "https://customer.example")
		listed := preflight(t, env.Server, path, "https://app.example")

		assert.Empty(t, other.Get("Access-Control-Allow-Origin"), path)
		assert.Equal(t, "https://app.example", listed.Get("Access-Control-Allow-Origin"), path)
		assert.Equal(t, "true", listed.Get("Access-Control-Allow-Credentials"), path)
	}
}

// crossOriginCase is one request the cross-origin guard judges.
type crossOriginCase struct {
	name, method, path, site string
}

// An unsafe request from another site to a cookie-authenticated path never reaches its handler.
func TestTheCrossOriginGuardRefusesACrossSiteWriteToACookiePath(t *testing.T) {
	env := testhelper.Setup(t)

	for _, c := range []crossOriginCase{
		{"cross-site site write", http.MethodPost, "/site/workspaces", "cross-site"},
		{"cross-site login", http.MethodPost, "/site/auth/login", "cross-site"},
		{"cross-site sign up", http.MethodPost, "/site/auth/sign-up", "cross-site"},
		{"same-site subdomain write", http.MethodDelete, "/site/workspaces/AcmeWs01", "same-site"},
	} {
		rec := serveRequest(t, env.Server, c.method, c.path, map[string]string{"Sec-Fetch-Site": c.site})

		assert.Equal(t, http.StatusForbidden, rec.Code, c.name)
	}
}

// A read, a same-origin or non-browser write, and every surface that is cross-origin by design
// (collect, one-click unsubscribe, the external API, OAuth) pass the guard to their handler.
func TestTheCrossOriginGuardPassesEverythingElse(t *testing.T) {
	env := testhelper.Setup(t)

	for _, c := range []crossOriginCase{
		{"same-origin sign up", http.MethodPost, "/site/auth/sign-up", "same-origin"},
		{"same-origin site write", http.MethodPost, "/site/workspaces", "same-origin"},
		{"cross-site read", http.MethodGet, "/site/workspaces", "cross-site"},
		{"non-browser write", http.MethodPost, "/site/workspaces", ""},
		{"collect", http.MethodPost, "/collect/events", "cross-site"},
		{"one-click unsubscribe", http.MethodPost, "/e/u/token", "cross-site"},
		{"external API", http.MethodPost, "/api/contacts", "cross-site"},
		{"OAuth token", http.MethodPost, "/oauth/token", "cross-site"},
	} {
		rec := serveRequest(t, env.Server, c.method, c.path, map[string]string{"Sec-Fetch-Site": c.site})

		assert.NotEqual(t, http.StatusForbidden, rec.Code, c.name)
	}
}

// The app origin (SPHERICON_APP_URL may carry a trailing slash or a path; only its origin counts)
// and the credentialed-CORS allowlist may write cross-origin; any other origin may not.
func TestTheCrossOriginGuardTrustsTheAppAndAllowlistedOrigins(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithConfig(func(c *config.Config) {
		c.AppURL = "https://sphericon.example/"
		c.CORSOrigins = []string{"https://app.example"}
	}))

	for origin, want := range map[string]int{
		"https://sphericon.example": http.StatusUnauthorized, // past the guard, at the session check
		"https://app.example":       http.StatusUnauthorized,
		"https://evil.example":      http.StatusForbidden,
	} {
		rec := serveRequest(t, env.Server, http.MethodPost, "/site/workspaces", map[string]string{
			"Sec-Fetch-Site": "cross-site", "Origin": origin,
		})

		assert.Equal(t, want, rec.Code, origin)
	}
}

func TestAnAppURLThatIsNoOriginFailsTheServer(t *testing.T) {
	_, err := server.New(&config.Config{AppURL: "not a url"}, nil, nil, nil, apisite.Deps{}, nil, http.NotFoundHandler(), http.NotFoundHandler(), nil)

	assert.ErrorContains(t, err, "trusted origin")
}

func TestADevelopmentServerShowsTheFaultOfAPanic(t *testing.T) {
	env := testhelper.Setup(t, development())
	panicOnVisitorWrites(env, "boom")

	rec := postCollect(t, env, collectReq{key: fixtures.AcmeCollectKey})

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	debug := debugOf(t, rec)
	assert.Equal(t, "boom", debug.Error)
	assert.Contains(t, debug.Stack, "TestADevelopmentServerShowsTheFaultOfAPanic")
	assert.Equal(t, http.MethodPost, debug.Method)
	assert.Equal(t, collectEvents, debug.Path)
}

func TestADevelopmentServerShowsTheCauseOfAReturnedError(t *testing.T) {
	env := testhelper.Setup(t, development())
	failVisitorWrites(env, errors.New("db password is hunter2"))

	rec := postCollect(t, env, collectReq{key: fixtures.AcmeCollectKey})

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, debugOf(t, rec).Error, "db password is hunter2")
}

func TestAPanicOfAProductionServerCarriesNoDebugBlock(t *testing.T) {
	env := testhelper.Setup(t, production())
	panicOnVisitorWrites(env, "boom")

	rec := postCollect(t, env, collectReq{key: fixtures.AcmeCollectKey})

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.NotContains(t, rec.Body.String(), "debug")
	assert.NotContains(t, rec.Body.String(), "boom", "the cause stays in the log")
}
