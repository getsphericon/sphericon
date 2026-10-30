package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/getsphericon/sphericon/internal/inuse"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/validation"
)

// The refusals every surface answers alike (review rule 15): a handler returns the module's error
// and refusalProblem turns it into the problem, so no handler maps them itself. The core error
// types know nothing of HTTP; this is where they meet a status. A module's sentinel refusal
// (validation.RefusalError) carries its kind, and the kind decides the status and the code: not
// found 404, refused 422, conflict 409, forbidden 403 with code permission_required, unauthorized
// 401, denied 403, unverified 403 with code email_verification_required, a pending acceptance 403
// with code acceptance_pending, a withheld User 403 with code second_factor_required, an invalid
// link 400, incomplete 428 and an unknown reference (a pending sign-up that is gone, a foreign
// cursor) 400. The bodies are the problem models of typespec/common, which every contract shares,
// so the site's generated types encode them for every surface.

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
	// A refused User deletion: its answer always lists the Workspaces the User solely owns.
	if refused, ok := errors.AsType[*accounts.DeletionRefusedError](err); ok {
		status := kindStatus(refused.Reason.Kind())
		return refusal{status: status, body: deletionBody(status, refused)}, true
	}
	if form, ok := errors.AsType[*validation.FieldsError](err); ok {
		p := problemBody(http.StatusUnprocessableEntity, form.Detail())
		p.Errors = siteapi.NewOptProblemDetailsErrors(siteapi.ProblemDetailsErrors(form.Fields))
		return refusal{status: http.StatusUnprocessableEntity, body: p}, true
	}
	if invalid, ok := errors.AsType[*validation.Error](err); ok {
		return refusal{status: http.StatusUnprocessableEntity, body: fieldBody(http.StatusUnprocessableEntity, invalid.Field, invalid.Message)}, true
	}
	// A module's sentinel refusal: its kind decides the status. The detail is the message of the
	// outermost refusal in the chain, so a refusal made With a detail shows it.
	if refused, ok := errors.AsType[*validation.RefusalError](err); ok {
		status := kindStatus(refused.Kind())
		p := fieldBody(status, refused.Field(), validation.Message(err))
		if code, ok := kindCode(refused.Kind()); ok {
			p.Code = siteapi.NewOptProblemCode(code)
		}
		return refusal{status: status, body: p}, true
	}
	// Content that cannot render is the author's to fix; the error carries the reason.
	if errors.Is(err, emailrender.ErrInvalidContent) {
		return refusal{status: http.StatusUnprocessableEntity, body: problemBody(http.StatusUnprocessableEntity, err.Error())}, true
	}
	return refusal{}, false
}

// kindStatus is the status every surface answers a refusal of kind with.
func kindStatus(kind validation.Kind) int {
	switch kind {
	case validation.KindNotFound:
		return http.StatusNotFound
	case validation.KindConflict:
		return http.StatusConflict
	case validation.KindForbidden:
		return http.StatusForbidden
	case validation.KindUnauthorized:
		return http.StatusUnauthorized
	case validation.KindDenied, validation.KindUnverified, validation.KindAcceptancePending, validation.KindSecondFactorRequired:
		return http.StatusForbidden
	case validation.KindLinkInvalid, validation.KindUnknownReference:
		return http.StatusBadRequest
	case validation.KindIncomplete:
		return http.StatusPreconditionRequired
	case validation.KindRefused:
		return http.StatusUnprocessableEntity
	}
	return http.StatusUnprocessableEntity
}

// kindCode is the problem code a refusal of kind carries, by which the client tells it from the
// other answers of its status: a Role without the Permission, an address not proven yet, a required
// acceptance pending and a User the Two-factor requirement withholds.
func kindCode(kind validation.Kind) (siteapi.ProblemCode, bool) {
	switch kind {
	case validation.KindForbidden:
		return siteapi.ProblemCodePermissionRequired, true
	case validation.KindUnverified:
		return siteapi.ProblemCodeEmailVerificationRequired, true
	case validation.KindAcceptancePending:
		return siteapi.ProblemCodeAcceptancePending, true
	case validation.KindSecondFactorRequired:
		return siteapi.ProblemCodeSecondFactorRequired, true
	default:
		return "", false
	}
}

// fieldBody is the problem of a refusal that names the request field it is about, or the plain
// problem when field is "".
func fieldBody(status int, field, detail string) *siteapi.ProblemDetails {
	p := problemBody(status, detail)
	if field != "" {
		p.Errors = siteapi.NewOptProblemDetailsErrors(siteapi.ProblemDetailsErrors{field: {detail}})
	}
	return p
}

// problemBody is the plain problem of a refusal: its status, the status text as title, and detail.
func problemBody(status int, detail string) *siteapi.ProblemDetails {
	return &siteapi.ProblemDetails{
		Status: siteapi.NewOptInt32(narrow.Int32(status)),
		Title:  siteapi.NewOptString(http.StatusText(status)),
		Detail: siteapi.NewOptString(detail),
	}
}

// deletionBody is the contract's SiteDeleteUserProblemDetails of a refused User deletion (ADR
// 0050): the rule's message, the field it is about, and the Workspaces the User solely owns
// (none for every rule but the sole Owner's).
func deletionBody(status int, e *accounts.DeletionRefusedError) *siteapi.SiteDeleteUserProblemDetails {
	base := fieldBody(status, e.Reason.Field(), e.Reason.Message())
	p := &siteapi.SiteDeleteUserProblemDetails{
		Status: base.Status,
		Title:  base.Title,
		Detail: base.Detail,
		Workspaces: lo.Map(e.Workspaces, func(w *ent.Workspace, _ int) siteapi.SiteSoleOwnedWorkspace {
			return siteapi.SiteSoleOwnedWorkspace{ID: siteapi.WorkspaceId(w.PublicID), Name: w.Name}
		}),
	}
	if errs, ok := base.Errors.Get(); ok {
		p.Errors = siteapi.NewOptSiteDeleteUserProblemDetailsErrors(siteapi.SiteDeleteUserProblemDetailsErrors(errs))
	}
	return p
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
