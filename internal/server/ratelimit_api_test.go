package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/getsphericon/sphericon/config"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	apiMe       = "/api/auth/me"
	badBearer   = "omtk_deadbeef_invalidsecret"
	guesserAddr = "203.0.113.50"
)

func getAPI(t *testing.T, env *testhelper.TestEnv, bearer string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, apiMe, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	env.Server.ServeHTTP(rec, req)
	return rec
}

func anchorBearer() string {
	return credentials.TokenValue(fixtures.AnchorTokenPrefix, fixtures.AnchorTokenSecret)
}

func remaining(t *testing.T, rec *httptest.ResponseRecorder) int {
	t.Helper()
	n, err := strconv.Atoi(rec.Header().Get("X-RateLimit-Remaining"))
	require.NoError(t, err, "X-RateLimit-Remaining")
	return n
}

func TestAPIAnswers429OverTheSteadyWindow(t *testing.T) {
	const limit = 3
	env := testhelper.Setup(t, testhelper.WithRateLimits(config.RateLimits{APIPerMinute: limit}))
	for i := range limit {
		rec := getAPI(t, env, anchorBearer(), nil)
		require.Equal(t, http.StatusOK, rec.Code, "request %d", i+1)
	}
	rec := getAPI(t, env, anchorBearer(), nil)
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	assert.Contains(t, rec.Body.String(), `"status":429`)
	assert.Equal(t, "60", rec.Header().Get("Retry-After"))
	assert.Equal(t, strconv.Itoa(limit), rec.Header().Get("X-RateLimit-Limit"))
	assert.Equal(t, "0", rec.Header().Get("X-RateLimit-Remaining"))
}

func TestAPIAnswers429OverTheBurstWindow(t *testing.T) {
	const burst = 3
	env := testhelper.Setup(t, testhelper.WithRateLimits(config.RateLimits{APIBurst: burst, APIPerMinute: 1000}))
	for i := range burst {
		require.Equal(t, http.StatusOK, getAPI(t, env, anchorBearer(), nil).Code, "request %d", i+1)
	}
	rec := getAPI(t, env, anchorBearer(), nil)
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, "1", rec.Header().Get("Retry-After"))
}

func TestSuccessfulResponsesCarryTheRateLimitHeaders(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithRateLimits(config.RateLimits{APIBurst: 20, APIPerMinute: 600}))
	rec := getAPI(t, env, anchorBearer(), nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "600", rec.Header().Get("X-RateLimit-Limit"))
	assert.Equal(t, "599", rec.Header().Get("X-RateLimit-Remaining"))
	assert.NotEmpty(t, rec.Header().Get("X-RateLimit-Reset"))
	assert.Empty(t, rec.Header().Get("Retry-After"))
}

func TestTokensOfOneWorkspaceShareTheBudgetAndAnotherWorkspaceIsUnaffected(t *testing.T) {
	const limit = 2
	env := testhelper.Setup(t, testhelper.WithRateLimits(config.RateLimits{APIPerMinute: limit}))
	second := env.ScopedBearer(t, "contacts:read")
	globex := env.ScopedBearerFor(t, fixtures.GlobexID, "contacts:read")

	require.Equal(t, http.StatusOK, getAPI(t, env, anchorBearer(), nil).Code)
	require.Equal(t, http.StatusOK, getAPI(t, env, second, nil).Code)
	assert.Equal(t, http.StatusTooManyRequests, getAPI(t, env, anchorBearer(), nil).Code)
	assert.Equal(t, http.StatusTooManyRequests, getAPI(t, env, second, nil).Code)

	assert.Equal(t, http.StatusOK, getAPI(t, env, globex, nil).Code)
}

func whoami(session *mcp.ClientSession) error {
	_, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "whoami"})
	return err
}

func TestMCPAndAPIDrawFromOneBudgetAndAnMCPCallCostsOne(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithRateLimits(config.RateLimits{APIPerMinute: 30}))
	session := env.MCPClient(t, env.ScopedBearer(t, "contacts:read"))

	before := getAPI(t, env, anchorBearer(), nil)
	require.Equal(t, http.StatusOK, before.Code)
	require.NoError(t, whoami(session))
	after := getAPI(t, env, anchorBearer(), nil)
	require.Equal(t, http.StatusOK, after.Code)

	// One MCP tool call is one HTTP request (it is authenticated again in-process
	// by /api and by the send lock, but charged once), plus the second /api call.
	assert.Equal(t, 2, remaining(t, before)-remaining(t, after))
}

// spendBudget calls the API until a call is refused with 429 (or attempts calls have been made) and
// returns the last answer.
func spendBudget(t *testing.T, env *testhelper.TestEnv, attempts int) *httptest.ResponseRecorder {
	t.Helper()
	var rec *httptest.ResponseRecorder
	for range attempts {
		if rec = getAPI(t, env, anchorBearer(), nil); rec.Code == http.StatusTooManyRequests {
			break
		}
	}
	return rec
}

func TestMCPAnswers429WhenTheWorkspaceBudgetIsSpent(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithRateLimits(config.RateLimits{APIPerMinute: 8}))
	session := env.MCPClient(t, env.ScopedBearer(t, "contacts:read"))

	rec := spendBudget(t, env, 20)
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.Error(t, whoami(session))
}

func TestFailedBearerAuthenticationsArePerIPLimitedAndSuccessesAreNotCounted(t *testing.T) {
	const limit = 3
	env := testhelper.Setup(t, testhelper.WithRateLimits(config.RateLimits{FailedAuth: limit}))
	from := func(ip string) map[string]string { return map[string]string{"X-Forwarded-For": ip} }

	for range 10 {
		require.Equal(t, http.StatusOK, getAPI(t, env, anchorBearer(), from(guesserAddr)).Code)
	}
	for i := range limit {
		require.Equal(t, http.StatusUnauthorized, getAPI(t, env, badBearer, from(guesserAddr)).Code, "failure %d", i+1)
	}
	rec := getAPI(t, env, badBearer, from(guesserAddr))
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))
	assert.NotEmpty(t, rec.Header().Get("X-RateLimit-Limit"), "the contract requires the whole header set on a 429")
	assert.NotEmpty(t, rec.Header().Get("X-RateLimit-Remaining"))
	assert.NotEmpty(t, rec.Header().Get("X-RateLimit-Reset"))

	// The guesser is cut off even with a good token; another address is not.
	assert.Equal(t, http.StatusTooManyRequests, getAPI(t, env, anchorBearer(), from(guesserAddr)).Code)
	assert.Equal(t, http.StatusOK, getAPI(t, env, anchorBearer(), from("203.0.113.51")).Code)
}

func postMCP(t *testing.T, env *testhelper.TestEnv, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, testhelper.MCPEndpoint, nil)
	req.Header.Set("Authorization", "Bearer "+bearer)
	rec := httptest.NewRecorder()
	env.Server.ServeHTTP(rec, req)
	return rec
}

func TestFailedMCPAuthenticationsAreLimitedToo(t *testing.T) {
	const limit = 2
	env := testhelper.Setup(t, testhelper.WithRateLimits(config.RateLimits{FailedAuth: limit}))
	for range limit {
		require.Equal(t, http.StatusUnauthorized, postMCP(t, env, badBearer).Code)
	}
	assert.Equal(t, http.StatusTooManyRequests, postMCP(t, env, badBearer).Code)
}

func TestAZeroAPILimitNeverThrottles(t *testing.T) {
	env := testhelper.Setup(t)
	for range 50 {
		require.Equal(t, http.StatusOK, getAPI(t, env, anchorBearer(), nil).Code)
	}
	assert.Empty(t, getAPI(t, env, anchorBearer(), nil).Header().Get("X-RateLimit-Limit"))
}

// The generated external client types the 429, so a caller needs no string matching.
func TestTheGeneratedExternalClientTypesTheRateLimitResponse(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithRateLimits(config.RateLimits{APIPerMinute: 1}))
	client := env.ExternalAnchor(t)

	_, err := client.AuthMeGet(t.Context())
	require.NoError(t, err)
	res, err := client.AuthMeGet(t.Context())
	require.NoError(t, err)

	limited, ok := res.(*externalapi.ProblemDetailsHeaders)
	require.True(t, ok, "got %T", res)
	assert.Equal(t, int32(60), limited.RetryAfter)
	assert.Equal(t, int32(1), limited.XRateLimitLimit)
	assert.Equal(t, int32(http.StatusTooManyRequests), limited.Response.Status.Value)
}
