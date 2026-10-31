package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/server"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The job queue's UI serves its embedded pages and its API from the binary, and lists jobs with
// their arguments hidden unless the Staff member asks for them.
func TestTheJobQueueUIServesItsPagesAndHidesArguments(t *testing.T) {
	ui, err := server.NewJobsUI(testhelper.RealDB(t))
	require.NoError(t, err)
	ctx, stop := context.WithCancel(context.Background())
	t.Cleanup(stop)
	require.NoError(t, ui.Start(ctx))

	page := serve(t, ui, "/staff/jobs/")
	assert.Equal(t, http.StatusOK, page.Code)
	assert.Contains(t, page.Header().Get("Content-Type"), "text/html")

	features := serve(t, ui, "/staff/jobs/api/features")
	require.Equal(t, http.StatusOK, features.Code)
	var body struct {
		HideArgs bool `json:"job_list_hide_args_by_default"`
	}
	require.NoError(t, json.Unmarshal(features.Body.Bytes(), &body))
	assert.True(t, body.HideArgs)
}

func serve(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
	return rec
}
