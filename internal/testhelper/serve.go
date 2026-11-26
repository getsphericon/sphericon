package testhelper

import (
	"net/http"
	"net/http/httptest"
)

// Serve sends one request through the composed server in memory and returns what it answered: for
// a test of the raw HTTP surface (a header, a redirect, a cookie, a body the typed clients do not
// expose). A test of an API operation asks for an actor instead.
func (env *TestEnv) Serve(req *http.Request) *httptest.ResponseRecorder {
	return ServeHandler(env.Server, req)
}

// ServeHandler sends one request through h in memory and returns what it answered: for a test of a
// handler or a middleware on its own, outside the composed server.
func ServeHandler(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
