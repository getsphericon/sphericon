//go:build !embed_spa

package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Without an embedded frontend (the default build) the catch-all answers with a hint.
func TestTheCatchAllOfABuildWithoutTheFrontendAnswersWithAHint(t *testing.T) {
	env := testhelper.Setup(t)

	rec := httptest.NewRecorder()
	env.Server.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))

	assert.Equal(t, http.StatusNotImplemented, rec.Code)
	assert.Contains(t, rec.Body.String(), "embed_spa")
}
