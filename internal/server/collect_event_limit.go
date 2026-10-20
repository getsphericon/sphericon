package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// collectEventLimit enforces the per-event cap inside a /collect/events batch
// before the ogen decoder runs (ADR 0025): each element of "events" may be at most
// limit bytes as sent. The body is already capped as a whole by bodyLimit, so it is
// read once here and handed back to ogen. Size is the element's exact serialized
// length. A body that does not parse is passed through untouched: ogen answers the
// 400, there is nothing to cap. The batch cap and the cap of a single event
// (identify) stay with bodyLimit.
func collectEventLimit(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.URL.Path != collectPrefix+"/events" || r.Body == nil {
				next.ServeHTTP(w, r)
				return
			}
			raw, err := io.ReadAll(r.Body)
			if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
				writeProblem(w, http.StatusRequestEntityTooLarge, err.Error())
				return
			}
			if err == nil && eventOverLimit(raw, limit) {
				writeProblem(w, http.StatusRequestEntityTooLarge,
					(&http.MaxBytesError{Limit: limit}).Error())
				return
			}
			// Hand ogen exactly what was read, even on a read error (it surfaces there).
			r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(raw), errReader{err}))
			next.ServeHTTP(w, r)
		})
	}
}

// eventOverLimit reports whether any element of the top-level "events" array is
// longer than limit bytes. Anything unparsable reports false.
func eventOverLimit(body []byte, limit int64) bool {
	dec := json.NewDecoder(bytes.NewReader(body))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return false
	}
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return false
		}
		if key != "events" {
			var skip json.RawMessage
			if dec.Decode(&skip) != nil {
				return false
			}
			continue
		}
		if tok, err := dec.Token(); err != nil || tok != json.Delim('[') {
			return false
		}
		for dec.More() {
			var event json.RawMessage
			if dec.Decode(&event) != nil {
				return false
			}
			if int64(len(event)) > limit {
				return true
			}
		}
		return false
	}
	return false
}

// errReader replays the error that ended a read (nil reads as end of body).
type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) {
	if e.err != nil {
		return 0, e.err
	}
	return 0, io.EOF
}
