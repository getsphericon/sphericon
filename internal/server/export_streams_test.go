package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

var exportPaths = []string{
	"/api/export/events",
	"/api/export/messages",
	"/api/export/contacts",
	"/api/export/audit",
	"/api/export/erasures",
}

func getExport(t *testing.T, env *testhelper.TestEnv, bearer, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
	req.Header.Set("Authorization", "Bearer "+bearer)
	rec := httptest.NewRecorder()
	env.Server.ServeHTTP(rec, req)
	return rec
}

func TestExportStreamsRefuseALimitOutsideTheContract(t *testing.T) {
	env := testhelper.Setup(t)
	bearer := env.ScopedBearerFor(t, fixtures.InitechID, "export:read")
	for _, path := range exportPaths {
		for _, limit := range []string{"0", "-5", "10001"} {
			rec := getExport(t, env, bearer, path+"?limit="+limit)

			assert.Equal(t, http.StatusBadRequest, rec.Code, "%s limit=%s", path, limit)
		}
		rec := getExport(t, env, bearer, path+"?limit=10000")
		assert.Equal(t, http.StatusOK, rec.Code, "%s limit=10000 is the largest page", path)
	}
}

func TestExportStreamsAreRateLimitedPerWorkspace(t *testing.T) {
	const limit = 2
	env := testhelper.Setup(t, testhelper.WithRateLimits(config.RateLimits{APIPerMinute: limit}))
	bearer := env.ScopedBearerFor(t, fixtures.InitechID, "export:read")
	for i := range limit {
		rec := getExport(t, env, bearer, "/api/export/events")
		require.Equal(t, http.StatusOK, rec.Code, "request %d", i+1)
	}

	rec := getExport(t, env, bearer, "/api/export/events")

	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, "60", rec.Header().Get("Retry-After"))
}
