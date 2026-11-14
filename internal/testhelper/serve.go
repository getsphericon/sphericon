package testhelper

import (
	"net/http"
	"net/http/httptest"
)

// Serve sends one request through the composed server in memory and returns what it answered: for
// a test of the raw HTTP surface (a header, a redirect, a cookie, a body the typed clients do not
// expose). A test of an API operation asks for an actor instead.
func (env *TestEnv) Serve(req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	env.Server.ServeHTTP(rec, req)
	return rec
}
