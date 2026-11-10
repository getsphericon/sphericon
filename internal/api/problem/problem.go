// Package problem answers every refused or failed request of the ogen surfaces with the
// contract's Problem (RFC 7807), the `default` response every operation declares. ogen reduces
// that response to one error type per surface (ogen.yml, convenient errors): a handler returns
// its success type or an error, and the surface's NewError hands the error to the function of
// this package named after it (Site, External, ...), which ogen then encodes. The decoding
// failures ogen meets before a handler runs reach the server's error handler, which renders them
// with Of too, so a request is answered alike wherever it failed.
//
// The status and the body are decided here once for every surface (review rule 15): a module's
// refusal (refusal.go), a handler error that knows its own problem (Error), a rate limit (a
// module's refusal with a wait among them), a
// render that could not run, ogen's own errors and the faults that answer a 500.
package problem

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/ogen-go/ogen/ogenerrors"

	"github.com/getsphericon/sphericon/ent"
	collectapi "github.com/getsphericon/sphericon/gen/collect"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	staffapi "github.com/getsphericon/sphericon/gen/staff"
	staffeeapi "github.com/getsphericon/sphericon/gen/staffee"
	"github.com/getsphericon/sphericon/internal/api/problem/mapping"
	dbpkg "github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/logging"
	"github.com/getsphericon/sphericon/internal/infra/ratelimit"
	"github.com/getsphericon/sphericon/internal/kernel/emailrender"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/kernel/narrow"
)

// RenderRetryAfter is the Retry-After (seconds) of a 503 for a document render that could not run.
const RenderRetryAfter = "5"

// CollectRetryAfter is the Retry-After (seconds) of a /collect 503: long enough for a restarting
// database to come back, short enough that a client's events stay fresh.
const CollectRetryAfter = "5"

// Error is a handler's answer that no refusal kind gives (the 410 of an expired confirmation
// link): its status and its detail. A refusal a
// module decides is the module's sentinel, never an Error (ast-grep rule
// no-hand-written-problem-in-handlers).
type Error struct {
	Status int
	Detail string
}

func (e *Error) Error() string { return e.Detail }

// Problem is the status and the detail the request is answered with.
func (e *Error) Problem() (status int, detail string) { return e.Status, e.Detail }

// Known is an error a handler or a security handler returns that carries the problem it is
// answered with: the HTTP status and the localized detail (a token without a scope, Error).
type Known interface {
	error
	Problem() (status int, detail string)
}

type exchangeKey struct{}

// exchange is the request a problem answers and its writer: the log line names the request, a
// development server's debug block reads both, and a rate limit has put its headers on the writer.
type exchange struct {
	w http.ResponseWriter
	r *http.Request
}

// Middleware keeps the request and its writer for Of. It wraps every ogen server, so the
// in-process dispatch of /mcp has them too.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), exchangeKey{}, &exchange{w: w, r: r})))
	})
}

// Of is the status and the body err is answered with, and it logs the answer: a refusal at warn
// level with its detail, a server fault at error level, whose detail the client never sees.
func Of(ctx context.Context, err error) (int, *siteapi.ProblemDetails) {
	ex, _ := ctx.Value(exchangeKey{}).(*exchange)
	status, body := decide(ctx, ex, err)
	method, path := "", ""
	if ex != nil {
		method, path = ex.r.Method, logging.RedactedPath(ex.r.URL.Path)
	}
	if status >= http.StatusInternalServerError {
		slog.ErrorContext(ctx, "request failed", "method", method, "path", path, "error", err)
	} else {
		logging.FromContext(ctx).WarnContext(ctx, "request refused", "method", method, "path", path, "status", status, "detail", err.Error())
	}
	return status, body
}

func decide(ctx context.Context, ex *exchange, err error) (int, *siteapi.ProblemDetails) {
	// A budget a module keeps refused with a wait: answered like every rate limit, its headers set,
	// counted and logged here, where the request's writer is.
	if wait, ok := errors.AsType[*ratelimit.WaitError](err); ok {
		err = ratelimit.FromContext(ctx).Delay(ctx, wait.Policy, wait.Limit, wait.Wait, wait.At)
	}
	// A rate limit rejection already set its headers on the shared writer and was counted and
	// logged where it was decided (ratelimit.LimitedError); the body repeats the wait.
	if _, ok := errors.AsType[*ratelimit.LimitedError](err); ok {
		p := Body(http.StatusTooManyRequests, "rate limit exceeded, retry later")
		if ex != nil {
			if secs, err := strconv.Atoi(ex.w.Header().Get("Retry-After")); err == nil {
				p.RetryAfter = siteapi.NewOptInt32(narrow.Int32(secs))
			}
		}
		return http.StatusTooManyRequests, p
	}
	// A handler error that knows its own problem (a token without a scope) is answered as it says.
	if known, ok := errors.AsType[Known](err); ok {
		status, detail := known.Problem()
		return status, Body(status, detail)
	}
	// A refusal of a module that every surface answers alike (review rule 15).
	if status, body, ok := refusal(err); ok {
		return status, body
	}
	// A render that could not run (the request ended, a limit of ours stopped it) is no fault of
	// the document and no crash: the client may try again.
	if errors.Is(err, emailrender.ErrRenderUnavailable) {
		if ex != nil {
			ex.w.Header().Set("Retry-After", RenderRetryAfter)
		}
		return http.StatusServiceUnavailable, Body(http.StatusServiceUnavailable, emailrender.ErrRenderUnavailable.Error())
	}
	code := http.StatusInternalServerError
	var oe ogenerrors.Error
	var tooBig *http.MaxBytesError
	switch {
	case errors.As(err, &tooBig):
		code = http.StatusRequestEntityTooLarge
	case errors.As(err, &oe):
		code = oe.Code()
	case errors.Is(err, lifecycle.ErrInvalidTransition):
		// A move the entity's lifecycle refuses, or lost to a concurrent writer (ADR 0046).
		code = http.StatusConflict
	case errors.Is(err, ent.ErrNotInWorkspace):
		// A reference id from another Workspace (the scoped client's ErrNotInWorkspace, ADR 0017).
		code = http.StatusUnprocessableEntity
	}
	// A value ent's validators refuse is not here: they are a backstop behind the module's own
	// checks (ADR 0107), so one that fires is a missed check, a fault like a CHECK constraint.
	p := &siteapi.ProblemDetails{
		Status: siteapi.NewOptInt32(narrow.Int32(code)),
		Title:  siteapi.NewOptString(http.StatusText(code)),
	}
	if code < http.StatusInternalServerError {
		// The client gets this text as the problem's detail; the log keeps it beside the access line.
		p.Detail = siteapi.NewOptString(err.Error())
	} else if ex != nil {
		// The client is told nothing about a server fault, so the log is the only place it shows.
		if dbg := Debug(ex.r, ex.w, err.Error(), ""); dbg != nil {
			p.Debug = siteapi.NewOptProblemDebug(*dbg)
		}
	}
	return code, p
}

// Body is the plain problem of status: the status, its text as title, and detail.
func Body(status int, detail string) *siteapi.ProblemDetails {
	return &siteapi.ProblemDetails{
		Status: siteapi.NewOptInt32(narrow.Int32(status)),
		Title:  siteapi.NewOptString(http.StatusText(status)),
		Detail: siteapi.NewOptString(detail),
	}
}

// ErrorHandler renders, as the contract's Problem, what fails in an ogen server before or around a
// handler (a request it cannot decode), and the refusals of /mcp's own token check. A handler's
// own error goes through its surface's NewError instead, which answers through the same Of.
func ErrorHandler(ctx context.Context, w http.ResponseWriter, _ *http.Request, err error) {
	status, body := Of(ctx, err)
	Write(w, status, body)
}

// Write renders body as application/problem+json with status.
func Write(w http.ResponseWriter, status int, body *siteapi.ProblemDetails) {
	data, err := body.MarshalJSON()
	if err != nil {
		// The generated encoders do not fail on these bodies; a failure still answers the status.
		slog.Warn("encode problem", "error", err)
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	if _, err := w.Write(data); err != nil {
		slog.Warn("write json response", "error", err)
	}
}

// mapper maps what a refusal names to the problem models' members, and the site's problem to
// the other surfaces' (the goverter converter of converter.go).
var mapper mapping.Converter = &mapping.ConverterImpl{}

// Site is the NewError of /site.
func Site(ctx context.Context, err error) *siteapi.ProblemDetailsStatusCode {
	status, body := Of(ctx, err)
	return &siteapi.ProblemDetailsStatusCode{StatusCode: status, Response: *body}
}

// External is the NewError of /api.
func External(ctx context.Context, err error) *externalapi.ProblemDetailsStatusCode {
	status, body := Of(ctx, err)
	return &externalapi.ProblemDetailsStatusCode{StatusCode: status, Response: mapper.External(body)}
}

// Collect is the NewError of /collect, which also tells its client to retry: a database that
// cannot be reached is a 503 with Retry-After, because a retried event is counted once
// (clientEventId) and a client can send it again safely.
func Collect(ctx context.Context, err error) *collectapi.ProblemDetailsStatusCode {
	status, body := http.StatusServiceUnavailable, Body(http.StatusServiceUnavailable, "database unavailable, retry later")
	if dbpkg.IsUnavailable(err) {
		if ex, ok := ctx.Value(exchangeKey{}).(*exchange); ok {
			ex.w.Header().Set("Retry-After", CollectRetryAfter)
		}
	} else {
		status, body = Of(ctx, err)
	}
	return &collectapi.ProblemDetailsStatusCode{StatusCode: status, Response: mapper.Collect(body)}
}

// Staff is the NewError of the core Staff surface.
func Staff(ctx context.Context, err error) *staffapi.ProblemDetailsStatusCode {
	status, body := Of(ctx, err)
	return &staffapi.ProblemDetailsStatusCode{StatusCode: status, Response: mapper.Staff(body)}
}

// Staffee is the NewError of the Staff console's Enterprise sections.
func Staffee(ctx context.Context, err error) *staffeeapi.ProblemDetailsStatusCode {
	status, body := Of(ctx, err)
	return &staffeeapi.ProblemDetailsStatusCode{StatusCode: status, Response: mapper.Staffee(body)}
}

// Service is the NewError of the service endpoints.
func Service(ctx context.Context, err error) *serviceapi.ProblemDetailsStatusCode {
	status, body := Of(ctx, err)
	return &serviceapi.ProblemDetailsStatusCode{StatusCode: status, Response: mapper.Service(body)}
}
