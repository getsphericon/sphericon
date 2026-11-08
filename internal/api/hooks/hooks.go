// Package hooks is the Ingest hook (ADR 0037), a driver beside the API surfaces (ADR 0111): it
// serves the inbound provider callbacks `POST /hooks/{key}`, hands each one to the Callback
// interpretation (internal/usecase/callbackinterpretation) and answers what became of it. A bad
// signature or a wrong source is 4xx; our own failure is 5xx, so the provider retries; a valid
// request the Preset does not handle is 2xx with nothing recorded.
package hooks

import (
	"errors"
	"io"
	"net/http"

	"github.com/getsphericon/sphericon/internal/api/problem"
	"github.com/getsphericon/sphericon/internal/domain/sending/preset"
	"github.com/getsphericon/sphericon/internal/usecase/callbackinterpretation"
)

// New is the handler of `POST /hooks/{key}`.
func New(ingest *callbackinterpretation.UseCase) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /hooks/{key}", func(w http.ResponseWriter, r *http.Request) {
		// Bounded to what the engine parses, so an oversized body is never held in memory.
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, preset.MaxBodyBytes))
		if err != nil {
			if tooBig, ok := errors.AsType[*http.MaxBytesError](err); ok {
				write(w, http.StatusRequestEntityTooLarge, tooBig.Error())
				return
			}
			write(w, http.StatusBadRequest, "read body")
			return
		}
		err = ingest.Receive(r.Context(), callbackinterpretation.Callback{
			Key: r.PathValue("key"), Query: r.URL.RawQuery, Header: r.Header, Body: body,
		})
		switch {
		case err == nil:
			w.WriteHeader(http.StatusOK)
		case errors.Is(err, callbackinterpretation.ErrNoHook):
			http.NotFound(w, r)
		case errors.Is(err, preset.ErrUnauthentic):
			write(w, http.StatusForbidden, "the request is not authentic")
		case errors.Is(err, preset.ErrUnavailable):
			write(w, http.StatusBadGateway, "the request could not be verified right now")
		case errors.Is(err, preset.ErrMalformed):
			write(w, http.StatusBadRequest, "the request is malformed")
		case errors.Is(err, preset.ErrTransformFailed):
			write(w, http.StatusInternalServerError, "the request could not be read")
		case errors.Is(err, callbackinterpretation.ErrUnconfirmed):
			write(w, http.StatusBadGateway, "subscription confirmation failed")
		default:
			write(w, http.StatusInternalServerError, "processing failed")
		}
	})
	return mux
}

func write(w http.ResponseWriter, status int, detail string) {
	problem.Write(w, status, problem.Body(status, detail))
}
