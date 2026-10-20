package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getsphericon/sphericon/config"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/ogen-go/ogen/ogenerrors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func do(t *testing.T, h http.Handler, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestProbesOnlyAnswerGet(t *testing.T) {
	db, err := sql.Open("pgx", "postgres://127.0.0.1:1/none")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	for name, h := range map[string]http.Handler{"healthz": healthzHandler(), "readyz": readyzHandler(db)} {
		rec := do(t, h, http.MethodPost, "/"+name, nil)
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code, name)
		assert.Equal(t, http.MethodGet, rec.Header().Get("Allow"), name)
	}
}

func TestReadyzIsUnavailableWhenTheDatabaseIsUnreachable(t *testing.T) {
	db, err := sql.Open("pgx", "postgres://127.0.0.1:1/none?connect_timeout=1")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	rec := do(t, readyzHandler(db), http.MethodGet, "/readyz", nil)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
}

func TestProblemErrorHandlerRendersOgenErrorsAsProblemJSON(t *testing.T) {
	// A client error keeps its status and explains itself.
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/x", nil)
	rec := httptest.NewRecorder()
	problemErrorHandler(t.Context(), rec, req, &ogenerrors.DecodeRequestError{Err: errors.New("bad body")})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	assert.Contains(t, rec.Body.String(), `"title":"Bad Request"`)
	assert.Contains(t, rec.Body.String(), "decode request: bad body")

	// Anything else is a 500 that never leaks the cause.
	rec = httptest.NewRecorder()
	problemErrorHandler(t.Context(), rec, req, errors.New("db password is hunter2"))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.JSONEq(t, `{"status":500,"title":"Internal Server Error"}`, rec.Body.String())
}

func TestRecovererTurnsAPanicIntoA500(t *testing.T) {
	h := recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	rec := do(t, h, http.MethodGet, "/x", nil)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
}

func TestRequestIDIsEchoedOrGenerated(t *testing.T) {
	h := requestID(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	assert.Equal(t, "abc-123", do(t, h, http.MethodGet, "/", map[string]string{"X-Request-Id": "abc-123"}).Header().Get("X-Request-Id"))
	generated := do(t, h, http.MethodGet, "/", nil).Header().Get("X-Request-Id")
	assert.Len(t, generated, 26)
	assert.NotEqual(t, generated, do(t, h, http.MethodGet, "/", nil).Header().Get("X-Request-Id"))
}

func TestCORSPoliciesByPath(t *testing.T) {
	ok := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	preflight := func(h http.Handler, path string) http.Header {
		return do(t, h, http.MethodOptions, path, map[string]string{
			"Origin":                        "https://customer.example",
			"Access-Control-Request-Method": "POST",
		}).Header()
	}

	// The collect API echoes any origin but never allows credentials.
	for _, origins := range [][]string{nil, {"https://app.example"}} {
		h := corsMiddleware(origins)(ok)
		got := preflight(h, "/collect/v1/track")
		assert.Equal(t, "https://customer.example", got.Get("Access-Control-Allow-Origin"))
		assert.Empty(t, got.Get("Access-Control-Allow-Credentials"))
	}

	// Cookie-authenticated paths allow cross-origin credentials only for an explicit
	// allowlist: with none configured there are no CORS headers at all.
	for _, path := range []string{"/site/workspaces", "/site/auth/login"} {
		none := preflight(corsMiddleware(nil)(ok), path)
		assert.Empty(t, none.Get("Access-Control-Allow-Origin"), path)
		assert.Empty(t, none.Get("Access-Control-Allow-Credentials"), path)

		listed := corsMiddleware([]string{"https://app.example"})(ok)
		assert.Empty(t, preflight(listed, path).Get("Access-Control-Allow-Origin"), path)
		allowed := do(t, listed, http.MethodOptions, path, map[string]string{
			"Origin": "https://app.example", "Access-Control-Request-Method": "POST",
		}).Header()
		assert.Equal(t, "https://app.example", allowed.Get("Access-Control-Allow-Origin"), path)
		assert.Equal(t, "true", allowed.Get("Access-Control-Allow-Credentials"), path)
	}

	// Bearer-token surfaces (external API, MCP, OAuth) reflect any origin but never
	// allow credentials, whatever the allowlist.
	for _, origins := range [][]string{nil, {"https://app.example"}} {
		for _, path := range []string{"/api/v1/contacts", "/mcp", "/oauth/token"} {
			got := preflight(corsMiddleware(origins)(ok), path)
			assert.Equal(t, "https://customer.example", got.Get("Access-Control-Allow-Origin"), path)
			assert.Empty(t, got.Get("Access-Control-Allow-Credentials"), path)
		}
	}
}

func TestCrossOriginGuardProtectsCookiePathsOnly(t *testing.T) {
	var reached bool
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusNoContent)
	})
	guard, err := crossOriginGuard("https://sphericon.example", nil)
	require.NoError(t, err)
	h := guard(ok)

	cases := []struct {
		name, method, path, site string
		want                     int
	}{
		{"cross-site site write", http.MethodPost, "/site/workspaces", "cross-site", http.StatusForbidden},
		{"cross-site login", http.MethodPost, "/site/auth/login", "cross-site", http.StatusForbidden},
		{"cross-site register", http.MethodPost, "/site/auth/register", "cross-site", http.StatusForbidden},
		{"same-origin register", http.MethodPost, "/site/auth/register", "same-origin", http.StatusNoContent},
		{"same-site subdomain write", http.MethodDelete, "/site/workspaces/AcmeWs01", "same-site", http.StatusForbidden},
		{"same-origin site write", http.MethodPost, "/site/workspaces", "same-origin", http.StatusNoContent},
		{"cross-site read", http.MethodGet, "/site/workspaces", "cross-site", http.StatusNoContent},
		{"non-browser write", http.MethodPost, "/site/workspaces", "", http.StatusNoContent},
		{"collect is cross-origin by design", http.MethodPost, "/collect/v1/track", "cross-site", http.StatusNoContent},
		{"one-click unsubscribe", http.MethodPost, "/e/u/token", "cross-site", http.StatusNoContent},
		{"external API", http.MethodPost, "/api/v1/contacts", "cross-site", http.StatusNoContent},
		{"OAuth token", http.MethodPost, "/oauth/token", "cross-site", http.StatusNoContent},
	}
	for _, c := range cases {
		reached = false
		rec := do(t, h, c.method, c.path, map[string]string{"Sec-Fetch-Site": c.site})
		assert.Equal(t, c.want, rec.Code, c.name)
		assert.Equal(t, c.want == http.StatusNoContent, reached, c.name)
	}
}

func TestBodyLimitCapsCollectSeparatelyAndRendersProblem413(t *testing.T) {
	h := bodyLimit(config.BodyLimits{Default: 8, Collect: 4, CollectEvent: 2})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			problemErrorHandler(r.Context(), w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	post := func(path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, strings.NewReader(body)))
		return rec
	}

	assert.Equal(t, http.StatusNoContent, post("/api/x", "12345678").Code)
	rec := post("/api/x", "123456789")
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))

	// A collect batch and a single collect event have caps of their own.
	assert.Equal(t, http.StatusNoContent, post("/collect/events", "1234").Code)
	assert.Equal(t, http.StatusRequestEntityTooLarge, post("/collect/events", "12345").Code)
	assert.Equal(t, http.StatusNoContent, post("/collect/identify", "12").Code)
	assert.Equal(t, http.StatusRequestEntityTooLarge, post("/collect/identify", "123").Code)
}

func TestCrossOriginGuardTrustsAppAndAllowlistedOrigins(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	// SPHERICON_APP_URL may carry a trailing slash or path; only its origin counts.
	guard, err := crossOriginGuard("https://sphericon.example/", []string{"https://app.example"})
	require.NoError(t, err)
	h := guard(ok)

	for origin, want := range map[string]int{
		"https://sphericon.example": http.StatusNoContent,
		"https://app.example":       http.StatusNoContent,
		"https://evil.example":      http.StatusForbidden,
	} {
		rec := do(t, h, http.MethodPost, "/site/workspaces", map[string]string{
			"Sec-Fetch-Site": "cross-site", "Origin": origin,
		})
		assert.Equal(t, want, rec.Code, origin)
	}

	_, err = crossOriginGuard("not a url", nil)
	assert.Error(t, err)
}

func TestDevelopmentServerShowsTheFaultOfAPanic(t *testing.T) {
	h := devDebug(recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") })))
	rec := do(t, h, http.MethodGet, "/x", nil)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	var body struct {
		Debug problemDebug `json:"debug"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "boom", body.Debug.Error)
	assert.Contains(t, body.Debug.Stack, "TestDevelopmentServerShowsTheFaultOfAPanic")
	assert.Equal(t, http.MethodGet, body.Debug.Method)
	assert.Equal(t, "/x", body.Debug.Path)
}

func TestDevelopmentServerShowsTheCauseOfAReturnedError(t *testing.T) {
	h := devDebug(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		problemErrorHandler(r.Context(), w, r, errors.New("db password is hunter2"))
	}))
	rec := do(t, h, http.MethodPost, "/x", nil)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), `"error":"db password is hunter2"`)
}

func TestAPanicOfAProductionServerCarriesNoDebugBlock(t *testing.T) {
	h := recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	rec := do(t, h, http.MethodGet, "/x", nil)
	assert.NotContains(t, rec.Body.String(), "debug")
	assert.NotContains(t, rec.Body.String(), "boom")
}
