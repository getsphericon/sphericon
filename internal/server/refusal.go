package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/samber/lo"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/inuse"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/validation"
)

// The refusals every surface answers alike (review rule 15): a handler returns the module's error
// and refusalProblem turns it into the problem, so no handler maps them itself. The core error
// types know nothing of HTTP; this is where they meet a status. The bodies are the problem models
// of typespec/common, which every contract shares, so the site's generated types encode them for
// every surface.

// refusal is a problem a refused request is answered with: its status and its encoded body.
type refusal struct {
	status int
	body   json.Marshaler
}

// write renders the refusal as application/problem+json.
func (r refusal) write(w http.ResponseWriter) {
	body, err := r.body.MarshalJSON()
	if err != nil {
		// The generated encoders do not fail on these bodies; a failure still answers the status.
		slog.Warn("encode refusal", "error", err)
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(r.status)
	if _, err := w.Write(body); err != nil {
		slog.Warn("write json response", "error", err)
	}
}

// refusalProblem is the problem err is answered with, or false when err is not one of the
// refusals the error handler renders.
func refusalProblem(err error) (refusal, bool) {
	if errors.Is(err, accounts.ErrWorkspaceNotFound) {
		// An unknown Workspace and one the caller is not a member of read the same.
		return refusal{status: http.StatusNotFound, body: problemBody(http.StatusNotFound, "workspace not found")}, true
	}
	if used, ok := errors.AsType[*inuse.Error](err); ok {
		return refusal{status: http.StatusConflict, body: inUseBody(used)}, true
	}
	if invalid, ok := errors.AsType[*validation.Error](err); ok {
		p := problemBody(http.StatusUnprocessableEntity, invalid.Message)
		p.Errors = siteapi.NewOptProblemDetailsErrors(siteapi.ProblemDetailsErrors{invalid.Field: {invalid.Message}})
		return refusal{status: http.StatusUnprocessableEntity, body: p}, true
	}
	return refusal{}, false
}

// problemBody is the plain problem of a refusal: its status, the status text as title, and detail.
func problemBody(status int, detail string) *siteapi.ProblemDetails {
	return &siteapi.ProblemDetails{
		Status: siteapi.NewOptInt32(narrow.Int32(status)),
		Title:  siteapi.NewOptString(http.StatusText(status)),
		Detail: siteapi.NewOptString(detail),
	}
}

// inUseBody is the contract's InUseProblemDetails of a removal refused because live work uses
// the row (ADR 0049): `usedBy` names the users, with a count only for an entry that stands for
// several rows.
func inUseBody(e *inuse.Error) *siteapi.InUseProblemDetails {
	base := problemBody(http.StatusConflict, e.Error())
	return &siteapi.InUseProblemDetails{
		Status: base.Status,
		Title:  base.Title,
		Detail: base.Detail,
		UsedBy: lo.Map(e.Users, func(u inuse.User, _ int) siteapi.UsedBy {
			by := siteapi.UsedBy{Kind: siteapi.UsedByKind(u.Kind), ID: siteapi.EntityId(u.ID), Name: u.Name}
			if u.Count > 0 {
				by.Count = siteapi.NewOptInt32(narrow.Int32(u.Count))
			}
			return by
		}),
	}
}
