package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/server"
)

// deadlineOf is how long a request to path may run under the timeouts of limits: what is left of
// its context's deadline when the handler starts.
func deadlineOf(t *testing.T, limits config.BodyLimits, path string) time.Duration {
	t.Helper()
	var left time.Duration
	h := server.Timeouts(limits)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		deadline, ok := r.Context().Deadline()
		require.True(t, ok, "every request has a deadline")
		left = time.Until(deadline)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, nil))
	return left
}

// The upload of a CSV import's file runs under a timeout that fits its own cap, not the 30 seconds
// of every other request: a 50 MiB file at a megabit a second takes seven minutes.
func TestTheUploadOfAnImportRunsUnderATimeoutFittingItsCap(t *testing.T) {
	limits := config.BodyLimits{Default: 1 << 20, CSVImport: 50 << 20}

	upload := deadlineOf(t, limits, "/site/workspaces/ws_1/contact-imports")
	other := deadlineOf(t, limits, "/site/workspaces/ws_1/contacts")

	assert.Greater(t, upload, 7*time.Minute)
	assert.Less(t, other, time.Minute, "the request timeout of the rest")
}
