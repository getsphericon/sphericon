package site

import (
	"net/http"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/narrow"
)

// The hand-written problems of the handlers that do not return a module's refusal yet. A
// refusal answers the same on every surface and is mapped once (review rule 15): a handler
// returns the module's sentinel and refusalProblem in internal/server writes the problem, so a
// new handler never calls these (ast-grep rule no-hand-written-problem-in-site).

// problem builds a ProblemDetails with status, title and detail.
func problem(code int, detail string) siteapi.ProblemDetails {
	return siteapi.ProblemDetails{
		Status: siteapi.NewOptInt32(narrow.Int32(code)),
		Title:  siteapi.NewOptString(http.StatusText(code)),
		Detail: siteapi.NewOptString(detail),
	}
}

// problemWithErrors builds a ProblemDetails with field-level validation errors.
func problemWithErrors(code int, detail string, fieldErrors map[string][]string) siteapi.ProblemDetails {
	p := problem(code, detail)
	p.Errors = siteapi.NewOptProblemDetailsErrors(siteapi.ProblemDetailsErrors(fieldErrors))
	return p
}
