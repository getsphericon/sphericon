package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func serveSPA(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
	return rec
}

func TestSPAFileHandler(t *testing.T) {
	sub := fstest.MapFS{
		"index.html":    {Data: []byte(`<html lang="{{APP_LOCALE}}"></html>`)},
		"assets/app.js": {Data: []byte(`console.log(1)`)},
	}
	h := spaFileHandler(sub, "en", true)

	// Real files are served as-is.
	rec := serveSPA(t, h, "/assets/app.js")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "console.log(1)", rec.Body.String())

	// "/", index.html, a deep link and a directory all return the locale-injected shell.
	for _, p := range []string{"/", "/index.html", "/workspaces/acme/contacts", "/assets"} {
		rec = serveSPA(t, h, p)
		require.Equal(t, http.StatusOK, rec.Code, p)
		assert.Equal(t, `<html lang="en"></html>`, rec.Body.String(), p)
		assert.Equal(t, "text/html; charset=utf-8", rec.Header().Get("Content-Type"), p)
	}
}

func TestSPAFileHandlerWithoutIndexIsAnError(t *testing.T) {
	rec := serveSPA(t, spaFileHandler(fstest.MapFS{"assets/app.js": {Data: []byte("x")}}, "en", true), "/deep/link")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
