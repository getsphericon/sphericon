package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getsphericon/sphericon/ent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A foreign reference id in a request body is a client error, never a 500
// (ADR 0017): the scoped client's ErrNotInWorkspace renders as 422 problem+json.
func TestProblemErrorHandlerMapsNotInWorkspaceTo422(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil)

	problemErrorHandler(t.Context(), w, r, fmt.Errorf("Segment: %w", ent.ErrNotInWorkspace))

	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.InDelta(t, http.StatusUnprocessableEntity, body["status"], 0)
	assert.Contains(t, body["detail"], "not in the workspace")
}
