package server_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/getsphericon/sphericon/config"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/telemetry"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const humanLimit = 3

// humanEndpoints are the public, human-driven site endpoints the "human" policy
// covers (ADR 0025). Each one is its own bucket per IP.
var humanEndpoints = map[string]string{
	"signup":          "/site/auth/register",
	"resend":          "/site/auth/resend-verification",
	"verify email":    "/site/auth/verify-email",
	"invitation":      "/site/invitations/some-token/accept",
	"consent confirm": "/site/confirmations/some-token",
}

func postJSON(t *testing.T, env *testhelper.TestEnv, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	env.Server.ServeHTTP(rec, req)
	return rec
}

func withHumanLimit(limit int) testhelper.Option {
	return testhelper.WithRateLimits(config.RateLimits{Human: limit})
}

func TestHumanEndpointsAnswer429OverTheLimit(t *testing.T) {
	for name, path := range humanEndpoints {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t, withHumanLimit(humanLimit))
			for i := range humanLimit {
				rec := postJSON(t, env, path, `{}`, nil)
				require.NotEqual(t, http.StatusTooManyRequests, rec.Code, "request %d", i+1)
			}
			rec := postJSON(t, env, path, `{}`, nil)
			require.Equal(t, http.StatusTooManyRequests, rec.Code)
			assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
			assert.Contains(t, rec.Body.String(), `"status":429`)
			assert.Equal(t, "60", rec.Header().Get("Retry-After"))
			assert.Equal(t, strconv.Itoa(humanLimit), rec.Header().Get("X-RateLimit-Limit"))
			assert.Equal(t, "0", rec.Header().Get("X-RateLimit-Remaining"))
			assert.NotEmpty(t, rec.Header().Get("X-RateLimit-Reset"))
		})
	}
}

func TestHumanLimitKeysOnTheTrustedProxyHopNotASpoofedHeader(t *testing.T) {
	env := testhelper.Setup(t, withHumanLimit(humanLimit))
	path := humanEndpoints["signup"]
	// The client rotates the left-hand X-Forwarded-For entry; the trusted proxy
	// appended the real address (rightmost), so every request is the same caller.
	for i := range humanLimit {
		rec := postJSON(t, env, path, `{}`, map[string]string{"X-Forwarded-For": "10.0.0." + strconv.Itoa(i) + ", 203.0.113.7"})
		require.NotEqual(t, http.StatusTooManyRequests, rec.Code)
	}
	rec := postJSON(t, env, path, `{}`, map[string]string{"X-Forwarded-For": "10.9.9.9, 203.0.113.7"})
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)

	// A different address behind the proxy has its own budget.
	other := postJSON(t, env, path, `{}`, map[string]string{"X-Forwarded-For": "10.9.9.9, 203.0.113.8"})
	assert.NotEqual(t, http.StatusTooManyRequests, other.Code)
}

func TestAZeroLimitNeverThrottles(t *testing.T) {
	env := testhelper.Setup(t, withHumanLimit(0))
	for range 200 {
		rec := postJSON(t, env, humanEndpoints["signup"], `{}`, nil)
		require.NotEqual(t, http.StatusTooManyRequests, rec.Code)
	}
}

func TestOperationalEndpointsAreNeverLimited(t *testing.T) {
	env := testhelper.Setup(t, withHumanLimit(1))
	for _, path := range []string{"/healthz", "/readyz", "/metrics", "/hooks/" + fixtures.AcmeIngestKey + "/ses"} {
		for range 5 {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			env.Server.ServeHTTP(rec, req)
			require.NotEqual(t, http.StatusTooManyRequests, rec.Code, path)
		}
	}
}

func TestARejectionCarriesCORSHeaders(t *testing.T) {
	env := testhelper.Setup(t, withHumanLimit(1), testhelper.WithConfig(func(c *config.Config) {
		c.CORSOrigins = []string{"https://app.example"}
	}))
	headers := map[string]string{"Origin": "https://app.example"}
	postJSON(t, env, humanEndpoints["signup"], `{}`, headers)
	rec := postJSON(t, env, humanEndpoints["signup"], `{}`, headers)
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, "https://app.example", rec.Header().Get("Access-Control-Allow-Origin"))
}

// The generated site client types the 429, so a caller needs no string matching.
func TestTheGeneratedClientTypesTheRateLimitResponse(t *testing.T) {
	env := testhelper.Setup(t, withHumanLimit(1))
	client := env.SiteAnonymous(t)
	input := &siteapi.SiteRegisterInput{Name: "Eve", Email: "eve@example.test"}

	_, err := client.SiteAuthRegister(t.Context(), input)
	require.NoError(t, err)
	res, err := client.SiteAuthRegister(t.Context(), input)
	require.NoError(t, err)

	limited, ok := res.(*siteapi.ProblemDetailsHeaders)
	require.True(t, ok, "got %T", res)
	assert.Equal(t, int32(60), limited.RetryAfter)
	assert.Equal(t, int32(1), limited.XRateLimitLimit)
	assert.Equal(t, int32(0), limited.XRateLimitRemaining)
	assert.Equal(t, int32(http.StatusTooManyRequests), limited.Response.Status.Value)
}

func TestARejectionIsCountedByPolicyAndLoggedWithoutTheEmail(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	testhelper.InstallOtel(t, func(ctx context.Context) (func(context.Context) error, error) {
		return telemetry.Setup(ctx, &config.Config{OtelServiceName: "sphericon-test"}, "test", telemetry.BuildInfo{})
	})

	env := testhelper.Setup(t, withHumanLimit(1))
	body := `{"name":"Eve","email":"eve@leak.example","password":"hunter2hunter2"}`
	postJSON(t, env, humanEndpoints["signup"], body, nil)
	rec := postJSON(t, env, humanEndpoints["signup"], body, nil)
	require.Equal(t, http.StatusTooManyRequests, rec.Code)

	scrape := httptest.NewRecorder()
	telemetry.MetricsHandler().ServeHTTP(scrape, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", nil))
	assert.Regexp(t, `ratelimit_rejected_total\{[^}]*policy="human"[^}]*\} 1`, scrape.Body.String())

	assert.Contains(t, logs.String(), `"level":"WARN"`)
	assert.Contains(t, logs.String(), `"policy":"human"`)
	assert.NotContains(t, logs.String(), "eve@leak.example")
}
