package problem

import (
	"context"
	"net/http"

	siteapi "github.com/getsphericon/sphericon/gen/site"
)

type devDebugKey struct{}

// DevDebug marks every request of a development server, so that the 500 it answers carries
// the fault (the Rails-style error page of the SPA reads it). It is installed outside the
// server's recoverer, which renders a panic. A production Instance never installs it, and a 500
// there tells the client nothing about the cause.
func DevDebug(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), devDebugKey{}, true)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Debug describes the failed request for a 500 body (the contract's ProblemDebug), or returns
// nil when the server is not a development one. stack is empty for an error a handler returned,
// and an empty stack or request id is left out.
func Debug(r *http.Request, w http.ResponseWriter, cause, stack string) *siteapi.ProblemDebug {
	if enabled, _ := r.Context().Value(devDebugKey{}).(bool); !enabled {
		return nil
	}
	dbg := &siteapi.ProblemDebug{Error: cause, Method: r.Method, Path: r.URL.Path}
	if stack != "" {
		dbg.Stack = siteapi.NewOptString(stack)
	}
	if id := w.Header().Get("X-Request-ID"); id != "" {
		dbg.RequestId = siteapi.NewOptString(id)
	}
	return dbg
}
