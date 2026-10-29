package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The failures below are injected into the write collect makes: its error handler leaves every
// failure but a lost database to the one every surface shares.

// A foreign reference id in a request body is a client error, never a 500 (ADR 0017): the
// scoped client's ErrNotInWorkspace renders as a 422 problem.
func TestAReferenceToAnotherWorkspaceIsA422(t *testing.T) {
	env := testhelper.Setup(t)
	failVisitorWrites(env, fmt.Errorf("Segment: %w", ent.ErrNotInWorkspace))

	rec := postCollect(t, env, collectReq{key: fixtures.AcmeCollectKey})

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.InDelta(t, http.StatusUnprocessableEntity, body["status"], 0)
	assert.Contains(t, body["detail"], "not in the workspace")
}

// A render that could not run is not the author's error and not a crash: it is a 503 the client
// may retry, and it does not call the document invalid.
func TestAnUnavailableRenderIsA503WithRetryAfter(t *testing.T) {
	env := testhelper.Setup(t)
	failVisitorWrites(env, fmt.Errorf("save: %w", emailrender.ErrRenderUnavailable))

	rec := postCollect(t, env, collectReq{key: fixtures.AcmeCollectKey})

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.InDelta(t, http.StatusServiceUnavailable, body["status"], 0)
	assert.Contains(t, body["detail"], "try again")
	assert.NotContains(t, body["detail"], "invalid")
}
