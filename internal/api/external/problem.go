package external

import (
	"net/http"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/narrow"
)

// The hand-written problem of the one answer no refusal kind gives (the 402 of a missing
// Enterprise license). A refusal answers the same on every surface and is mapped once (review
// rule 15): a handler returns the module's sentinel and refusalProblem in internal/server writes
// the problem, so a new handler never calls this (ast-grep rule no-hand-written-problem-in-external).

// problem builds a ProblemDetails value for the given status code and detail.
func problem(code int, detail string) externalapi.ProblemDetails {
	return externalapi.ProblemDetails{
		Status: externalapi.NewOptInt32(narrow.Int32(code)),
		Title:  externalapi.NewOptString(http.StatusText(code)),
		Detail: externalapi.NewOptString(detail),
	}
}
