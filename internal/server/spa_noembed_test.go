//go:build !embed_spa

package server

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Without an embedded frontend (the default build) the catch-all answers with a hint.
func TestSPAHandlerNotEmbeddedHint(t *testing.T) {
	rec := serveSPA(t, spaHandler(spaSettings{Locale: "en", RequireEmailVerification: true}), "/")

	assert.Equal(t, http.StatusNotImplemented, rec.Code)
	assert.Contains(t, rec.Body.String(), "embed_spa")
}
