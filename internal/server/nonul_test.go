package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContainsNULFindsTheByteAndTheEscape(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{"a clean document", `{"name":"Ada"}`, false},
		{"the escape", `{"name":"a\u0000b"}`, true},
		{"a raw NUL byte", "{\"name\":\"a\x00b\"}", true},
		{"an escaped backslash before u0000", `{"name":"a\\u0000b"}`, false},
		{"an escaped backslash, then the escape", `{"name":"a\\\u0000b"}`, true},
		{"another escape", `{"name":"line\nbreak A"}`, false},
		{"a short escape", `{"name":"\u000"}`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := containsNUL([]byte(tc.body))

			assert.Equal(t, tc.want, got)
		})
	}
}

func serveNoNUL(t *testing.T, target, body string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	var seen string
	handler := noNUL(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		seen = string(raw)
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, target, strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	return rec, seen
}

func TestNoNULPassesACleanRequestWithItsBodyIntact(t *testing.T) {
	rec, seen := serveNoNUL(t, "/api/tags?x=1", `{"name":"ok"}`)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.JSONEq(t, `{"name":"ok"}`, seen)
}

func TestNoNULRefusesANULInTheBodyTheQueryOrThePath(t *testing.T) {
	bodyRec, bodySeen := serveNoNUL(t, "/api/tags", `{"name":"a\u0000"}`)
	queryRec, querySeen := serveNoNUL(t, "/api/contacts?email=a%00", `{}`)
	pathRec, pathSeen := serveNoNUL(t, "/api/contacts/1%00", `{}`)

	assert.Equal(t, http.StatusBadRequest, bodyRec.Code)
	assert.Equal(t, http.StatusBadRequest, queryRec.Code)
	assert.Equal(t, http.StatusBadRequest, pathRec.Code)
	assert.Empty(t, bodySeen+querySeen+pathSeen, "the handler is never reached")
}
