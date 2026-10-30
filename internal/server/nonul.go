package server

import (
	"bytes"
	"io"
	"net/http"
	"strings"
)

// noNUL refuses a request that carries a NUL character, in its path, its query or a JSON body
// (as the raw byte or as the escape \u0000). PostgreSQL stores no NUL in text and cannot parse
// the escape into jsonb, so such a value would otherwise reach a query and fail it as a 500, or
// sit in a stored document until a later cast over it (an erasure's jsonb lookup) fails.
// Run it after bodyLimit: the body it reads is already capped.
func noNUL(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.ContainsRune(r.URL.Path, 0) || strings.Contains(r.URL.RawQuery, "%00") {
			writeProblem(w, http.StatusBadRequest, "NUL characters are not allowed")
			return
		}
		// ContentLength 0 is a request without a body (a GET): there is nothing to read.
		if r.ContentLength != 0 && r.Body != nil && r.Body != http.NoBody {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				ProblemErrorHandler(r.Context(), w, r, err)
				return
			}
			if containsNUL(body) {
				writeProblem(w, http.StatusBadRequest, "NUL characters are not allowed")
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
		}
		next.ServeHTTP(w, r)
	})
}

// containsNUL reports whether a JSON document holds a NUL byte or the escape \u0000. A backslash
// pairs with the character after it, so the text \\u0000 (an escaped backslash, then u0000) is not
// an escape.
func containsNUL(body []byte) bool {
	for i := 0; i < len(body); i++ {
		switch body[i] {
		case 0:
			return true
		case '\\':
			if bytes.HasPrefix(body[i+1:], []byte("u0000")) {
				return true
			}
			i++ // the escaped character, so a pair of backslashes is skipped as one
		}
	}
	return false
}
