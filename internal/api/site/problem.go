package site

import (
	"net/http"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/narrow"
)

// The hand-written problem of the answers no refusal kind gives (the 402 of a missing Enterprise
// license, the 410 of an expired confirmation link). A refusal answers the same on every surface
// and is mapped once (review rule 15): a handler returns the module's sentinel and refusalProblem
// in internal/server writes the problem, so a new handler never calls this (ast-grep rule
// no-hand-written-problem-in-site).

// problem builds a ProblemDetails with status, title and detail.
func problem(code int, detail string) siteapi.ProblemDetails {
	return siteapi.ProblemDetails{
		Status: siteapi.NewOptInt32(narrow.Int32(code)),
		Title:  siteapi.NewOptString(http.StatusText(code)),
		Detail: siteapi.NewOptString(detail),
	}
}
