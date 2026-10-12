package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getsphericon/sphericon/config"
	collectapi "github.com/getsphericon/sphericon/gen/collect"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	collectEvents   = "/collect/events"
	collectIdentify = "/collect/identify"
	pageOrigin      = "https://shop.example.com"
	tinyEvent       = `{"events":[{"visitorId":"v1","action":"viewed"}]}`
)

type collectReq struct {
	path, key, ip, origin, body string
}

func postCollect(t *testing.T, env *testhelper.TestEnv, c collectReq) *httptest.ResponseRecorder {
	t.Helper()
	if c.path == "" {
		c.path = collectEvents
	}
	if c.body == "" {
		c.body = tinyEvent
	}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, c.path, strings.NewReader(c.body))
	req.Header.Set("Content-Type", "application/json")
	if c.key != "" {
		req.Header.Set("x-collect-key", c.key)
	}
	if c.ip != "" {
		req.Header.Set("X-Forwarded-For", c.ip)
	}
	if c.origin != "" {
		req.Header.Set("Origin", c.origin)
	}
	rec := httptest.NewRecorder()
	env.Server.ServeHTTP(rec, req)
	return rec
}

func TestCollectAnswers429OverTheWorkspaceLimitAndAnotherWorkspaceIsUnaffected(t *testing.T) {
	const limit = 3
	env := testhelper.Setup(t, testhelper.WithRateLimits(config.RateLimits{Collect: limit}))
	for i := range limit {
		rec := postCollect(t, env, collectReq{key: fixtures.AcmeCollectKey})
		require.Equal(t, http.StatusNoContent, rec.Code, "request %d", i+1)
	}
	rec := postCollect(t, env, collectReq{key: fixtures.AcmeCollectKey})
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	assert.Equal(t, "60", rec.Header().Get("Retry-After"))
	assert.Equal(t, "3", rec.Header().Get("X-RateLimit-Limit"))
	assert.Equal(t, "0", rec.Header().Get("X-RateLimit-Remaining"))

	assert.Equal(t, http.StatusNoContent, postCollect(t, env, collectReq{key: fixtures.GlobexCollectKey}).Code)
}

func TestCollectAnswers429ForOneIPOverItsLimitWhileOthersAreServed(t *testing.T) {
	const limit = 2
	env := testhelper.Setup(t, testhelper.WithRateLimits(config.RateLimits{CollectIP: limit}))
	for range limit {
		require.Equal(t, http.StatusNoContent, postCollect(t, env, collectReq{key: fixtures.AcmeCollectKey, ip: "203.0.113.60"}).Code)
	}
	rec := postCollect(t, env, collectReq{key: fixtures.AcmeCollectKey, ip: "203.0.113.60"})
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))

	assert.Equal(t, http.StatusNoContent, postCollect(t, env, collectReq{key: fixtures.AcmeCollectKey, ip: "203.0.113.61"}).Code)
}

func TestRepeatedWrongCollectKeysFromOneIPAreRefused(t *testing.T) {
	const limit = 3
	env := testhelper.Setup(t, testhelper.WithRateLimits(config.RateLimits{FailedAuth: limit}))
	guess := collectReq{key: "omck_wrong", ip: guesserAddr}
	for i := range limit {
		require.Equal(t, http.StatusUnauthorized, postCollect(t, env, guess).Code, "failure %d", i+1)
	}
	require.Equal(t, http.StatusTooManyRequests, postCollect(t, env, guess).Code)

	// Even the right key is refused from the guesser; successes elsewhere are not counted.
	assert.Equal(t, http.StatusTooManyRequests, postCollect(t, env, collectReq{key: fixtures.AcmeCollectKey, ip: guesserAddr}).Code)
	for range limit + 2 {
		require.Equal(t, http.StatusNoContent, postCollect(t, env, collectReq{key: fixtures.AcmeCollectKey, ip: "203.0.113.62"}).Code)
	}
}

func TestCrossOrigin429And413CarryCORSHeaders(t *testing.T) {
	env := testhelper.Setup(t,
		testhelper.WithRateLimits(config.RateLimits{Collect: 1}),
		testhelper.WithConfig(func(c *config.Config) { c.BodyLimits.CollectEvent = 64 }),
	)
	require.Equal(t, http.StatusNoContent, postCollect(t, env, collectReq{key: fixtures.AcmeCollectKey, origin: pageOrigin}).Code)

	limited := postCollect(t, env, collectReq{key: fixtures.AcmeCollectKey, origin: pageOrigin})
	require.Equal(t, http.StatusTooManyRequests, limited.Code)
	assert.Equal(t, pageOrigin, limited.Header().Get("Access-Control-Allow-Origin"))

	big := postCollect(t, env, collectReq{path: collectIdentify, key: fixtures.GlobexCollectKey, origin: pageOrigin,
		body: `{"visitorId":"` + strings.Repeat("x", 100) + `"}`})
	require.Equal(t, http.StatusRequestEntityTooLarge, big.Code)
	assert.Equal(t, pageOrigin, big.Header().Get("Access-Control-Allow-Origin"))
}

func TestPayloadsOverTheEventAndBatchCapsGet413(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithConfig(func(c *config.Config) {
		c.BodyLimits.CollectEvent = 200
		c.BodyLimits.Collect = 600
	}))
	event := func(pad int) string {
		return `{"visitorId":"v1","action":"viewed","properties":{"p":"` + strings.Repeat("x", pad) + `"}}`
	}
	post := func(body string) int {
		return postCollect(t, env, collectReq{key: fixtures.AcmeCollectKey, body: body}).Code
	}

	assert.Equal(t, http.StatusNoContent, post(`{"events":[`+event(50)+`]}`))
	// One event over the per-event cap, inside a batch that is under the batch cap.
	assert.Equal(t, http.StatusRequestEntityTooLarge, post(`{"events":[`+event(50)+`,`+event(300)+`]}`))
	// Each event fits, the batch does not.
	assert.Equal(t, http.StatusRequestEntityTooLarge, post(`{"events":[`+strings.Repeat(event(50)+",", 7)+event(50)+`]}`))
	// A single identify body is one event-sized payload.
	assert.Equal(t, http.StatusRequestEntityTooLarge,
		postCollect(t, env, collectReq{path: collectIdentify, key: fixtures.AcmeCollectKey,
			body: `{"visitorId":"v1","traits":{"p":"` + strings.Repeat("x", 300) + `"}}`}).Code)
}

// The per-event cap counts the event's serialized bytes as sent: one at the cap is
// accepted, one over is refused, however the bytes split between keys, values and
// whitespace.
func TestPerEventCapInABatchIsExactInSerializedBytes(t *testing.T) {
	const eventCap = 200
	env := testhelper.Setup(t, testhelper.WithConfig(func(c *config.Config) {
		c.BodyLimits.CollectEvent = eventCap
		c.BodyLimits.Collect = 1000
	}))
	post := func(body string) int {
		return postCollect(t, env, collectReq{key: fixtures.AcmeCollectKey, body: body}).Code
	}
	const head = `{"visitorId":"v1","action":"viewed","properties":{"p":"x"}}`
	atCap := head[:len(head)-1] + strings.Repeat(" ", eventCap-len(head)) + "}"
	require.Len(t, atCap, eventCap)

	assert.Equal(t, http.StatusNoContent, post(`{"events":[`+atCap+`]}`))
	assert.Equal(t, http.StatusRequestEntityTooLarge, post(`{"events":[`+atCap[:len(atCap)-1]+` }]}`))
}

func TestAZeroCollectLimitNeverThrottles(t *testing.T) {
	env := testhelper.Setup(t)
	for range 30 {
		require.Equal(t, http.StatusNoContent, postCollect(t, env, collectReq{key: fixtures.AcmeCollectKey}).Code)
	}
}

// The generated collect client types the 429, so a caller needs no string matching.
func TestTheGeneratedCollectClientTypesTheRateLimitResponse(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithRateLimits(config.RateLimits{Collect: 1}))
	client := env.CollectAcme(t)

	_, err := client.CollectEventsCreate(t.Context(), &collectapi.CollectEventsInput{})
	require.NoError(t, err)
	res, err := client.CollectEventsCreate(t.Context(), &collectapi.CollectEventsInput{})
	require.NoError(t, err)

	limited, ok := res.(*collectapi.ProblemDetailsHeaders)
	require.True(t, ok, "got %T", res)
	assert.Equal(t, int32(60), limited.RetryAfter)
}
