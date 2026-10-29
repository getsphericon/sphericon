package server_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// bodyRecorder is a surface that keeps the body it was handed and answers 204.
type bodyRecorder struct {
	t    *testing.T
	seen string
}

func (b *bodyRecorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(r.Body)
	assert.NoError(b.t, err)
	b.seen = string(raw)
	w.WriteHeader(http.StatusNoContent)
}

// postThrough posts body to target through a server whose surface records what reaches it.
func postThrough(t *testing.T, target, body string) (rec *httptest.ResponseRecorder, seen string) {
	t.Helper()
	surface := &bodyRecorder{t: t}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, target, strings.NewReader(body))
	rec = httptest.NewRecorder()

	internalServer(t, surface).ServeHTTP(rec, req)

	return rec, surface.seen
}

// A JSON document is refused when it holds a NUL byte or the escape \u0000. A backslash pairs
// with the character after it, so the text \\u0000 (an escaped backslash, then u0000) passes.
func TestANULInAJSONBodyIsRefused(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"a clean document", `{"name":"Ada"}`, http.StatusNoContent},
		{"the escape", `{"name":"a\u0000b"}`, http.StatusBadRequest},
		{"a raw NUL byte", "{\"name\":\"a\x00b\"}", http.StatusBadRequest},
		{"an escaped backslash before u0000", `{"name":"a\\u0000b"}`, http.StatusNoContent},
		{"an escaped backslash, then the escape", `{"name":"a\\\u0000b"}`, http.StatusBadRequest},
		{"another escape", `{"name":"line\nbreak A"}`, http.StatusNoContent},
		{"a short escape", `{"name":"\u000"}`, http.StatusNoContent},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec, _ := postThrough(t, "/service/tags", tc.body)

			assert.Equal(t, tc.want, rec.Code)
		})
	}
}

func TestACleanRequestPassesWithItsBodyIntact(t *testing.T) {
	rec, seen := postThrough(t, "/service/tags?x=1", `{"name":"ok"}`)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.JSONEq(t, `{"name":"ok"}`, seen)
}

func TestANULInTheBodyTheQueryOrThePathNeverReachesTheHandler(t *testing.T) {
	bodyRec, bodySeen := postThrough(t, "/service/tags", `{"name":"a\u0000"}`)
	queryRec, querySeen := postThrough(t, "/service/contacts?email=a%00", `{}`)
	pathRec, pathSeen := postThrough(t, "/service/contacts/1%00", `{}`)

	assert.Equal(t, http.StatusBadRequest, bodyRec.Code)
	assert.Equal(t, http.StatusBadRequest, queryRec.Code)
	assert.Equal(t, http.StatusBadRequest, pathRec.Code)
	assert.Empty(t, bodySeen+querySeen+pathSeen, "the handler is never reached")
}
