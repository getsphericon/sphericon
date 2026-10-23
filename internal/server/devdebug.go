package server

import (
	"context"
	"net/http"
)

type devDebugKey struct{}

// devDebug marks every request of a development server, so that the 500 it answers carries
// the fault (the Rails-style error page of the SPA reads it). It is installed outside
// recoverer, which renders a panic. A production Instance never installs it, and a 500 there
// tells the client nothing about the cause.
func devDebug(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), devDebugKey{}, true)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// problemDebug is the `debug` member of a development server's 500 problem
// (ProblemDebug in the contract).
type problemDebug struct {
	Error     string `json:"error"`
	Stack     string `json:"stack,omitempty"`
	RequestID string `json:"requestId,omitempty"`
	Method    string `json:"method"`
	Path      string `json:"path"`
}

// debugProblem describes the failed request for a 500 body, or returns nil when the server
// is not a development one. stack is empty for an error a handler returned.
func debugProblem(r *http.Request, w http.ResponseWriter, cause, stack string) *problemDebug {
	if enabled, _ := r.Context().Value(devDebugKey{}).(bool); !enabled {
		return nil
	}
	return &problemDebug{
		Error:     cause,
		Stack:     stack,
		RequestID: w.Header().Get("X-Request-ID"),
		Method:    r.Method,
		Path:      r.URL.Path,
	}
}
